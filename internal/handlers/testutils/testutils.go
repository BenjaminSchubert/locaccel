package testutils

import (
	"net/http"
	"path"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
	tst "github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var TestLogger = tst.TestLogger

func NewClient(t *testing.T, logger *zerolog.Logger) *httpclient.Client {
	t.Helper()

	client := &http.Client{
		Timeout: time.Minute,
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

	cache, err := httpclient.NewCache(
		path.Join(t.TempDir(), "cache"),
		units.Bytes{Bytes: 1024 * 1024},
		units.Bytes{Bytes: 10 * 1024 * 1024},
		logger,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cache.Close())
	})

	return httpclient.New(client, cache, logger, false)
}
