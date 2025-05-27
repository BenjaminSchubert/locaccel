package npm_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/npm"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestInstallNpmPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}
	npm.RegisterHandler(
		"https://registry.npmjs.org/",
		"http",
		handler,
		testutils.NewClient(t, logger),
	)
	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(handler, "npm", logger, prometheus.NewPedanticRegistry()),
	)
	defer server.Close()

	cmd := exec.Command( //nolint:gosec
		"podman",
		"run",
		"--rm",
		"--interactive",
		"--network=host",
		"--dns=127.0.0.127",
		"--env=NPM_CONFIG_UPDATE_NOTIFIER=false",
		"--env=npm_config_registry="+server.URL,
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
}
