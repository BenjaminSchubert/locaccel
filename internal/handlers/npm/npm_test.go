package npm

import (
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

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
				"react",
				"@npmcli/promise-spawn",
			)
		},
		true,
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

		resp, err := http.DefaultClient.Do(req)
		require.NoError(b, err)

		npmInfo, err = io.ReadAll(resp.Body)
		require.NoError(b, err)
		require.NoError(b, resp.Body.Close())
	}

	r, err := http.NewRequestWithContext(b.Context(), http.MethodGet, "https://locaccel.test", nil)
	require.NoError(b, err)

	for b.Loop() {
		_, err := rewriteJson(npmInfo, r, "https://registry.npmjs.org/", "https", nil)
		require.NoError(b, err)
	}
}
