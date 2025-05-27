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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/admin"
	"github.com/benjaminschubert/locaccel/internal/handlers/goproxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/npm"
	"github.com/benjaminschubert/locaccel/internal/handlers/oci"
	"github.com/benjaminschubert/locaccel/internal/handlers/proxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/pypi"
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

func New(
	conf *config.Config,
	client *httpclient.Client,
	cache *httpclient.Cache,
	logger *zerolog.Logger,
	metricsRegistry interface {
		prometheus.Registerer
		prometheus.Gatherer
	},
) *Server {
	srv := Server{logger: logger}

	for _, proxy := range conf.GoProxies {
		srv.servers = append(
			srv.servers,
			setupGoProxy(conf, proxy, client, logger, metricsRegistry),
		)
	}

	for _, registry := range conf.OciRegistries {
		srv.servers = append(
			srv.servers,
			setupOciRegistry(conf, registry, client, logger, metricsRegistry),
		)
	}

	for _, registry := range conf.PyPIRegistries {
		srv.servers = append(
			srv.servers,
			setupPypiRegistry(conf, registry, client, logger, metricsRegistry),
		)
	}

	for _, registry := range conf.NpmRegistries {
		srv.servers = append(
			srv.servers,
			setupNpmRegistry(conf, registry, client, logger, metricsRegistry),
		)
	}

	for _, proxy := range conf.Proxies {
		srv.servers = append(srv.servers, setupProxy(conf, proxy, client, logger, metricsRegistry))
	}

	if conf.AdminInterface != "" {
		srv.servers = append(srv.servers, setupAdminInterface(conf, cache, logger, metricsRegistry))
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

func setupGoProxy(
	conf *config.Config,
	goProxy config.GoProxy,
	client *httpclient.Client,
	logger *zerolog.Logger,
	registry prometheus.Registerer,
) serverInfo {
	serviceName := "go[" + goProxy.Upstream + "]"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()
	goproxy.RegisterHandler(goProxy.Upstream, handler, client)

	return createServer(
		fmt.Sprintf("%s:%d", conf.Host, goProxy.Port),
		handler,
		serviceName,
		&log,
		registry,
	)
}

func setupOciRegistry(
	conf *config.Config,
	registry config.OciRegistry,
	client *httpclient.Client,
	logger *zerolog.Logger,
	metricsRegistry prometheus.Registerer,
) serverInfo {
	serviceName := "oci[" + registry.Upstream + "]"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()
	oci.RegisterHandler(registry.Upstream, handler, client)

	return createServer(
		fmt.Sprintf("%s:%d", conf.Host, registry.Port),
		handler,
		serviceName,
		&log,
		metricsRegistry,
	)
}

func setupPypiRegistry(
	conf *config.Config,
	registry config.PyPIRegistry,
	client *httpclient.Client,
	logger *zerolog.Logger,
	metricsRegistry prometheus.Registerer,
) serverInfo {
	serviceName := "pypi[" + registry.Upstream + "]"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()
	pypi.RegisterHandler(registry.Upstream, registry.CDN, handler, client)

	return createServer(
		fmt.Sprintf("%s:%d", conf.Host, registry.Port),
		handler,
		serviceName,
		&log,
		metricsRegistry,
	)
}

func setupNpmRegistry(
	conf *config.Config,
	registry config.NpmRegistry,
	client *httpclient.Client,
	logger *zerolog.Logger,
	metricsRegistry prometheus.Registerer,
) serverInfo {
	serviceName := "npm[" + registry.Upstream + "]"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()
	npm.RegisterHandler(registry.Upstream, registry.Scheme, handler, client)

	return createServer(
		fmt.Sprintf("%s:%d", conf.Host, registry.Port),
		handler,
		serviceName,
		&log,
		metricsRegistry,
	)
}

func setupProxy(
	conf *config.Config,
	proxyConf config.Proxy,
	client *httpclient.Client,
	logger *zerolog.Logger,
	registry prometheus.Registerer,
) serverInfo {
	serviceName := "proxy"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()
	proxy.RegisterHandler(proxyConf.AllowedUpstreams, handler, client)

	return createServer(
		fmt.Sprintf("%s:%d", conf.Host, proxyConf.Port),
		handler,
		serviceName,
		&log,
		registry,
	)
}

func setupAdminInterface(
	conf *config.Config,
	cache *httpclient.Cache,
	logger *zerolog.Logger,
	registry interface {
		prometheus.Registerer
		prometheus.Gatherer
	},
) serverInfo {
	serviceName := "admin"
	log := logger.With().Str("service", serviceName).Logger()

	handler := http.NewServeMux()

	if conf.EnableProfiling {
		log.Info().
			Str("profilingUrl", conf.AdminInterface+"/-/pprof/").
			Msg("Enabling profiling")
		handlers.RegisterProfilingHandlers(handler, "/-/pprof/")
	}

	if conf.EnableMetrics {
		log.Info().
			Str("metricsUrl", conf.AdminInterface+"/metrics").
			Msg("Enabling metrics")
		handler.Handle(
			"GET /metrics",
			promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	}

	if err := admin.RegisterHandler(handler, cache, conf); err != nil {
		logger.Panic().Err(err).Msg("unable to initialize server properly")
	}

	return createServer(conf.AdminInterface, handler, serviceName, &log, registry)
}

func createServer(
	address string,
	handler *http.ServeMux,
	serviceName string,
	log *zerolog.Logger,
	registry prometheus.Registerer,
) serverInfo {
	handler.HandleFunc("/", handlers.NotImplemented)

	return serverInfo{
		&http.Server{
			Addr:         address,
			Handler:      middleware.ApplyAllMiddlewares(handler, serviceName, log, registry),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 5 * time.Minute,
			ErrorLog:     stdlog.New(log, "", 0),
		},
		log,
	}
}
