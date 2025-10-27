package main

import (
	"net/http"
	"os"
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

func main() {
	panicLogger, err := logging.CreateLogger(zerolog.WarnLevel, "json", os.Stderr)
	if err != nil {
		panic("BUG: invalid default logger")
	}

	configPath, configPathSet := os.LookupEnv("LOCACCEL_CONFIG_PATH")
	if !configPathSet {
		var ok bool
		configPath, ok = os.LookupEnv("LOCACCEL_DEFAULT_CONFIG_PATH")
		if !ok {
			configPath = "./locaccel.yaml"
		}
	}

	conf, err := config.Parse(configPath, os.LookupEnv)
	if err != nil {
		if configPathSet {
			panicLogger.Fatal().Err(err).Msg("Unable to start server: invalid configuration")
		}
		conf = config.Default(os.LookupEnv)
	}

	logLevel, err := zerolog.ParseLevel(conf.Log.Level)
	if err != nil {
		panicLogger.Fatal().Err(err).Msg("Unable to start server: invalid configuration")
	}
	logger, err := logging.CreateLogger(logLevel, conf.Log.Format, os.Stderr)
	if err != nil {
		panicLogger.Fatal().Err(err).Msg("Unable to initialize logger")
	}

	logger.Info().Str("version", getVersion()).Msg("Running locaccel")
	if !configPathSet {
		logger.Info().
			Msg("locaccel.yaml not found and LOCACCEL_CONFIG_PATH not set: Using default configuration")
	}

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

	cache, err := httpclient.NewCache(conf.Cache.Path, quotaLow, quotaHigh, &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to start server: can't setup cache")
	}
	defer func() {
		logger.Info().Msg("Closing up the cache")
		if err := cache.Close(); err != nil {
			logger.Error().Err(err).Msg("Couldn't close the cache properly")
		}
	}()

	var registry *prometheus.Registry
	if conf.EnableMetrics {
		registry := prometheus.NewRegistry()
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}

	cachingClient := httpclient.New(
		client,
		cache,
		&logger,
		conf.Cache.Private,
		middleware.SetCacheState,
		time.Now,
		time.Since,
	)

	srv := server.New(conf, cachingClient, cache, &logger, registry)
	if err := srv.ListenAndServe(); err != nil {
		logger.Panic().Err(err).Msg("An error occurred while shutting down the server")
	}

	logger.Info().Msg("Server shut down")
}
