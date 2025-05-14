package server

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/oci"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

type serverInfo struct {
	server *http.Server
	logger *zerolog.Logger
}

type Server struct {
	servers []serverInfo
	logger  *zerolog.Logger
}

func New(conf *config.Config, client *httpclient.Client, logger *zerolog.Logger) *Server {
	srv := Server{logger: logger}

	for _, registry := range conf.OciRegistries {
		srv.servers = append(srv.servers, setupOciRegistry(conf, registry, client, logger))
	}

	if conf.AdminInterface != "" {
		srv.servers = append(srv.servers, setupAdminInterface(conf, logger))
	} else if conf.EnableProfiling {
		logger.Warn().Msg("Profiling requested, but the admin interface is disabled. Ignoring.")
	}

	return &srv
}

func (s *Server) ListenAndServe() error {
	errChan := make(chan error)
	defer close(errChan)

	for _, srv := range s.servers {
		go func() {
			srv.logger.Info().Str("address", srv.server.Addr).Msg("Starting server")
			err := srv.server.ListenAndServe()
			if !errors.Is(err, http.ErrServerClosed) {
				srv.logger.Error().Err(err).Msg("Server didn't come up properly")
				errChan <- err
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	select {
	case <-stop:
		s.logger.Info().Msg("Shutting down")
	case err := <-errChan:
		s.logger.Error().Err(err).Msg("At least one server is unhealthy, shutting down")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	closingErrs := make(chan error)
	defer close(closingErrs)

	for _, srv := range s.servers {
		go func() {
			err := srv.server.Shutdown(ctx)
			if err != nil {
				srv.logger.Error().Err(err).Msg("Error shutting down the server")
			}
			closingErrs <- err
		}()
	}

	var lastErr error
	for range len(s.servers) {
		if err := <-closingErrs; err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func setupOciRegistry(
	conf *config.Config,
	registry config.OciRegistry,
	client *httpclient.Client,
	logger *zerolog.Logger,
) serverInfo {
	handler := http.NewServeMux()
	oci.RegisterHandler(registry.Remote, handler, client)
	handler.HandleFunc("/", handlers.NotImplemented)

	log := logger.With().Str("service", "oci["+registry.Remote+"]").Logger()

	return serverInfo{
		&http.Server{
			Addr:         fmt.Sprintf("%s:%d", conf.Host, registry.Port),
			Handler:      middleware.ApplyAllMiddlewares(handler, &log),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 5 * time.Minute,
			ErrorLog:     stdlog.New(&log, "", 0),
		},
		&log,
	}
}

func setupAdminInterface(conf *config.Config, logger *zerolog.Logger) serverInfo {
	log := logger.With().Str("service", "admin").Logger()

	handler := http.NewServeMux()

	if conf.EnableProfiling {
		log.Info().
			Str("profilingUrl", conf.AdminInterface+"/-/pprof/").
			Msg("Enabling profiling")
		handlers.RegisterProfilingHandlers(handler, "/-/pprof/")
	}

	handler.HandleFunc("/", handlers.NotImplemented)

	return serverInfo{
		&http.Server{
			Addr:         conf.AdminInterface,
			Handler:      middleware.ApplyAllMiddlewares(handler, &log),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 5 * time.Minute,
			ErrorLog:     stdlog.New(&log, "", 0),
		},
		&log,
	}
}
