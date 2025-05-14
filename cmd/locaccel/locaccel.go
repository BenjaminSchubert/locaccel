package main

import (
	"net/http"
	"time"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/logging"
	"github.com/benjaminschubert/locaccel/internal/server"
)

func main() {
	logger := logging.CreateLogger()

	conf := config.New()

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
