package server

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func listen(t *testing.T, network, addr string) net.Listener {
	t.Helper()
	lc := net.ListenConfig{}
	listener, err := lc.Listen(t.Context(), network, addr)
	require.NoError(t, err)
	return listener
}

func TestServerInitialization(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)

	conf, err := config.Default(func(s string) (string, bool) { return "", false })
	require.NoError(t, err)
	conf.EnableProfiling = true
	srv := New(conf, nil, nil, logger, nil, &middleware.Statistics{})

	require.Len(t, srv.servers, 11)

	addresses := make([]string, 0, len(srv.servers))
	for _, s := range srv.servers {
		addresses = append(addresses, s.server.Addr)
	}
	require.Equal(
		t,
		[]string{
			"localhost:3147",
			"localhost:3143",
			"localhost:3131",
			"localhost:3132",
			"localhost:3133",
			"localhost:3134",
			"localhost:3145",
			"localhost:3144",
			"localhost:3142",
			"localhost:3146",
			"localhost:3130",
		},
		addresses,
	)
}

func TestListenAndServeMultipleServerFailures(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(t)))

	// Reserve a port, then force servers to use it to make them fail
	blocker := listen(t, "tcp", "localhost:0")
	t.Cleanup(func() { _ = blocker.Close() })
	blockedAddr := blocker.Addr().String()

	newFailedServer := func() serverInfo {
		log := logger.With().Str("service", "test").Logger()
		return serverInfo{
			server: &http.Server{
				Addr:              blockedAddr,
				Handler:           http.NotFoundHandler(),
				ReadHeaderTimeout: time.Second,
			},
			logger: &log,
		}
	}

	srv := &Server{
		logger: &logger,
		servers: []serverInfo{
			newFailedServer(),
			newFailedServer(),
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe()
	}()

	// With the bug, the second sender panics on the closed channel, which
	// crashes the whole test binary — the test fails loudly. With the fix,
	// ListenAndServe must terminate cleanly (nil or error, either is fine).
	select {
	case <-done:
		// Success: no deadlock, no panic.
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe did not terminate: goroutine likely blocked on errChan send")
	}
}

func TestListenAndServeShutdownWhileErrorPending(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(t)))

	// Reserve a port
	blocker := listen(t, "tcp", "localhost:0")
	t.Cleanup(func() { _ = blocker.Close() })
	blockedAddr := blocker.Addr().String()

	// The healthy server grabs a real port
	healthyListener := listen(t, "tcp", "localhost:0")
	t.Cleanup(func() { _ = healthyListener.Close() })

	healthy := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	go func() { _ = healthy.Serve(healthyListener) }()
	t.Cleanup(func() { _ = healthy.Close() })

	failLog := logger.With().Str("service", "failing").Logger()
	healthyLog := logger.With().Str("service", "healthy").Logger()

	failing := &http.Server{
		Addr:              blockedAddr,
		Handler:           http.NotFoundHandler(),
		ReadHeaderTimeout: time.Second,
	}

	srv := &Server{
		logger: &logger,
		servers: []serverInfo{
			{server: failing, logger: &failLog},
			{server: healthy, logger: &healthyLog},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe()
	}()

	select {
	case err := <-done:
		// Shutdown of the never-served failing server returns nil, so err is
		// expected to be nil here; the point is reaching this branch at all.
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe did not terminate: goroutine likely blocked on errChan send")
	}
}
