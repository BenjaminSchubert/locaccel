package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/logging"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	"github.com/benjaminschubert/locaccel/internal/server"
)

func getVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "<unknown>"
	}
	return info.Main.Version
}

func loadConfig(lookupEnv func(string) (string, bool)) (*config.Config, bool, error) {
	configPath, configPathSet := lookupEnv("LOCACCEL_CONFIG_PATH")
	if !configPathSet {
		var ok bool
		configPath, ok = lookupEnv("LOCACCEL_DEFAULT_CONFIG_PATH")
		if !ok {
			configPath = "./locaccel.yaml"
		}
	}

	conf, err := config.Parse(configPath, lookupEnv)
	configNotFound := errors.Is(err, fs.ErrNotExist)
	if configNotFound && !configPathSet {
		conf, err = config.Default(lookupEnv)
	}

	return conf, configNotFound, err
}

func startServer(conf *config.Config, logger *zerolog.Logger) {
	logger.Info().Str("version", getVersion()).Msg("Running locaccel")

	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			MaxConnsPerHost:       20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	quotaLow, err := conf.Cache.GetQuotaLow()
	if err != nil {
		logger.Fatal().Err(err).Msg("Unable to get low quota for the cache.")
	}
	quotaHigh, err := conf.Cache.GetQuotaHigh()
	if err != nil {
		logger.Fatal().Err(err).Msg("Unable to get low quota for the cache.")
	}

	cache, err := httpclient.NewCache(conf.Cache.Path, quotaLow, quotaHigh, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to start server: can't setup cache")
	}
	defer func() {
		logger.Info().Msg("Closing up the cache")
		if err := cache.Close(); err != nil {
			logger.Error().Err(err).Msg("Couldn't close the cache properly")
		}
	}()

	var registry interface {
		prometheus.Registerer
		prometheus.Gatherer
	}
	if conf.EnableMetrics {
		registry = prometheus.NewRegistry()
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}

	statsPath := path.Join(conf.Cache.Path, "statistics.json")
	stats, err := middleware.LoadSavedStatistics(statsPath, logger)
	if err != nil {
		logger.Panic().
			Err(err).
			Str("path", statsPath).
			Msg("Unable to load previous statistics. Please remove or fix the file")
	}
	defer func() {
		if err := stats.Save(statsPath, logger); err != nil {
			logger.Error().Err(err).Msg("Unable to save statistics about cache")
		} else {
			logger.Debug().Str("path", statsPath).Msg("Statistics saved to disk")
		}
	}()

	cachingClient := httpclient.New(
		client,
		cache,
		logger,
		conf.Cache.Private,
		middleware.SetCacheState,
		time.Now,
		time.Since,
	)

	srv := server.New(conf, cachingClient, cache, logger, registry, stats)
	if err := srv.ListenAndServe(); err != nil {
		logger.Panic().Err(err).Msg("An error occurred while shutting down the server")
	}

	logger.Info().Msg("Server shut down")
}

func healthCheck(conf *config.Config, logger *zerolog.Logger) {
	client := http.Client{
		Timeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://"+conf.AdminInterface+"/healthcheck",
		nil,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Unable to prepare request for healthcheck")
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.Fatal().Err(err).Msg("The /healthcheck endpoint is unreachable")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Panic().Err(err).Msg("unexpected error closing the response's body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Fatal().Int("status", resp.StatusCode).Msg("The system is not healthy")
	}
}

func getPanicLogger() *zerolog.Logger {
	panicLogger, err := logging.CreateLogger(zerolog.WarnLevel, "json", os.Stderr)
	if err != nil {
		panic("BUG: invalid default logger")
	}
	return &panicLogger
}

func main() {
	var runHealthcheck bool
	flag.BoolVar(
		&runHealthcheck,
		"healthcheck",
		false,
		"Run the healthcheck against the current locaccel instance",
	)
	flag.Parse()

	conf, configNotExist, err := loadConfig(os.LookupEnv)
	if err != nil {
		getPanicLogger().Fatal().Err(err).Msg("Error loading configuration")
	}

	logger, err := logging.CreateLogger(conf.Log.Level, conf.Log.Format, os.Stderr)
	if err != nil {
		getPanicLogger().Fatal().Err(err).Msg("Error initializing logger")
	}

	if !configNotExist {
		logger.Info().
			Msg("locaccel.yaml not found and LOCACCEL_CONFIG_PATH not set: Using default configuration")
	}

	if runHealthcheck {
		healthCheck(conf, &logger)
	} else {
		startServer(conf, &logger)
	}
}
