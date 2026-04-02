package npm

import (
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

var npmInfo []byte = nil

func TestInstallNpmPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"npm",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			RegisterHandler(
				"https://registry.npmjs.org/",
				"http",
				handler,
				client,
				upstreamCaches,
			)
		},
		func(t *testing.T, serverURL string) {
			t.Helper()

			testutils.Execute(
				t,
				"podman",
				"run",
				"--rm",
				"--interactive",
				"--network=host",
				"--dns=127.0.0.127",
				"--env=NPM_CONFIG_UPDATE_NOTIFIER=false",
				"--env=npm_config_registry="+serverURL,
				"node:slim",
				"npm",
				"pack",
				"--dry-run",
				"--loglevel",
				"silly",
				"@npmcli/promise-spawn",
			)
		},
		true,
		1,
	)
}

func BenchmarkJSONRewrite(b *testing.B) {
	if npmInfo == nil {
		req, err := http.NewRequestWithContext(
			b.Context(),
			http.MethodGet,
			"https://registry.npmjs.org/react",
			nil,
		)
		require.NoError(b, err)
		req.Header.Add("Accept", "application/vnd.npm.install-v1+json")

		resp, err := http.DefaultClient.Do(req) //nolint:gosec
		require.NoError(b, err)

		npmInfo, err = io.ReadAll(resp.Body)
		require.NoError(b, err)
		require.NoError(b, resp.Body.Close())
	}

	r, err := http.NewRequestWithContext(b.Context(), http.MethodGet, "https://locaccel.test", nil)
	require.NoError(b, err)

	jsonHandler := handlers.NewJSONHandler()

	for b.Loop() {
		err := rewriteJson(npmInfo, r, "https://registry.npmjs.org/", "https", nil, jsonHandler)
		require.NoError(b, err)
		jsonHandler.Buffer.Reset()
	}
}
