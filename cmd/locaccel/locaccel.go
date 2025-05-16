package main

import (
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/logging"
	"github.com/benjaminschubert/locaccel/internal/server"
)

func main() {
	panicLogger, err := logging.CreateLogger(zerolog.WarnLevel, "json")
	if err != nil {
		panic("BUG: invalid default logger")
	}

	configPath, configPathSet := os.LookupEnv("LOCACCEL_CONFIG_PATH")
	if !configPathSet {
		configPath = "./locaccel.yaml"
	}

	conf, err := config.Parse(configPath)
	if err != nil {
		if configPathSet {
			panicLogger.Fatal().Err(err).Msg("Unable to start server: invalid configuration")
		}
		conf = config.Default()
	}

	logLevel, err := zerolog.ParseLevel(conf.Log.Level)
	if err != nil {
		panicLogger.Fatal().Err(err).Msg("Unable to start server: invalid configuration")
	}
	logger, err := logging.CreateLogger(logLevel, conf.Log.Format)
	if err != nil {
		panicLogger.Fatal().Err(err).Msg("Unable to initialize logger")
	}

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
	cachingClient, err := httpclient.New(client, conf.CachePath, &logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to start server: can't setup cache")
	}
	defer func() {
		logger.Info().Msg("Closing up the cache")
		if err := cachingClient.Close(); err != nil {
			logger.Error().Err(err).Msg("Couldn't close the cache properly")
		}
	}()

	srv := server.New(conf, cachingClient, &logger)
	if err := srv.ListenAndServe(); err != nil {
		logger.Panic().Err(err).Msg("An error occurred while shutting down the server")
	}

	logger.Info().Msg("Server shut down")
}
