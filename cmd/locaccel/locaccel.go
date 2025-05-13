package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/oci"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/logging"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func createServer(
	conf config.Config,
	client *httpclient.Client,
	logger *zerolog.Logger,
) *http.Server {
	handler := http.NewServeMux()

	oci.RegisterHandler("https://registry-1.docker.io", handler, client)

	if os.Getenv("LOCACCEL_ENABLE_PROFILING") == "1" {
		logger.Warn().Msg("Profiling enabled under /-/pprof/")
		handlers.RegisterProfilingHandlers(handler, "/-/pprof/")
	}

	handler.HandleFunc("/", handlers.NotImplemented)

	return &http.Server{
		Addr:         conf.Address,
		Handler:      middleware.ApplyAllMiddlewares(handler, logger),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 5 * time.Minute,
	}
}

func runServer(server *http.Server, logger zerolog.Logger) error {
	isStopping := false

	go func() {
		logger.Info().Str("address", server.Addr).Msg("Starting server")

		if err2 := server.ListenAndServe(); err2 != nil {
			if !isStopping || !errors.Is(err2, http.ErrServerClosed) {
				logger.Fatal().Err(err2).Msg("Server failed")
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	isStopping = true
	return server.Shutdown(ctx)
}

func main() {
	logger := logging.CreateLogger()

	conf, err := config.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("Unable to start server: invalid config")
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

	server := createServer(conf, cachingClient, &logger)

	if err = runServer(server, logger); err != nil {
		logger.Panic().Err(err).Msg("Error shutting down server")
	}

	logger.Info().Msg("Server shut down")
}
