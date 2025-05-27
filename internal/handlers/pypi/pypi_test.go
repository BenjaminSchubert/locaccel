package pypi_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/pypi"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestInstallPythonPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}
	pypi.RegisterHandler(
		"https://pypi.org",
		"https://files.pythonhosted.org",
		handler,
		testutils.NewClient(t, logger),
	)
	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(handler, "pypi", logger, prometheus.NewPedanticRegistry()),
	)
	defer server.Close()

	cmd := exec.Command( //nolint:gosec
		"podman",
		"run",
		"--rm",
		"--interactive",
		"--network=host",
		"--dns=127.0.0.127",
		"--env=PIP_INDEX_URL="+server.URL+"/simple",
		"python:slim",
		"pip",
		"install",
		"--disable-pip-version-check",
		"uv",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}
