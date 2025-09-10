package npm_test

import (
	"net/http"
	"net/url"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/npm"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallNpmPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"npm",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			npm.RegisterHandler(
				"https://registry.npmjs.org/",
				"http",
				handler,
				client,
				upstreamCaches,
			)
		},
		func(t *testing.T, serverURL string) {
			t.Helper()

			cmd := exec.Command( //nolint:gosec
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
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		},
	)
}
