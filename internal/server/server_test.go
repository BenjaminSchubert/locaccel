package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func TestServerInitialization(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)

	conf, err := config.Default(func(s string) (string, bool) { return "", false })
	require.NoError(t, err)
	conf.EnableProfiling = true
	srv := New(conf, nil, nil, logger, nil, &middleware.Statistics{})

	require.Len(t, srv.servers, 10)

	addresses := make([]string, 0, len(srv.servers))
	for _, s := range srv.servers {
		addresses = append(addresses, s.server.Addr)
	}
	require.Equal(
		t,
		[]string{
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
