package goproxy_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/goproxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestInstallGoPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}
	goproxy.RegisterHandler("https://proxy.golang.org", handler, testutils.NewClient(t, logger))
	server := httptest.NewServer(middleware.ApplyAllMiddlewares(handler, logger))
	defer server.Close()

	cwd, err := os.Getwd()
	require.NoError(t, err)
	root := path.Dir(path.Dir(path.Dir(cwd)))

	cmd := exec.Command( //nolint:gosec
		"podman",
		"run",
		"--rm",
		"--interactive",
		"--network=host",
		"--dns=127.0.0.127",
		"--env=GOPROXY="+server.URL,
		"--env=GOSUMDB=sum.golang.org "+server.URL+"/sumdb",
		"--volume="+path.Join(root, "go.mod")+":/src/go.mod:Z,ro",
		"--volume="+path.Join(root, "go.sum")+":/src/go.sum:Z,ro",
		"--workdir=/src",
		"docker.io/golang:alpine",
		"go",
		"get",
		"./...",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}
