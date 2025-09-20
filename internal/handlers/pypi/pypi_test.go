package pypi_test

import (
	"net/http"
	"net/url"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/pypi"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallPythonPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"pypi",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			pypi.RegisterHandler(
				"https://pypi.org",
				"https://files.pythonhosted.org",
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
				"--env=PIP_INDEX_URL="+serverURL+"/simple",
				"python:slim",
				"pip",
				"install",
				"--disable-pip-version-check",
				"uv",
			)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		},
		true,
	)
}
