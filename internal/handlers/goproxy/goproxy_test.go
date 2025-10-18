package goproxy_test

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/goproxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallGoPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"go",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			goproxy.RegisterHandler("https://proxy.golang.org", handler, client, upstreamCaches)
		},
		func(t *testing.T, serverURL string) {
			t.Helper()

			cwd, err := os.Getwd()
			require.NoError(t, err)
			root := path.Dir(path.Dir(path.Dir(cwd)))

			testutils.Execute(
				t,
				"podman",
				"run",
				"--rm",
				"--interactive",
				"--network=host",
				"--dns=127.0.0.127",
				"--env=GOPROXY="+serverURL,
				"--env=GOSUMDB=sum.golang.org "+serverURL+"/sumdb",
				"--volume="+path.Join(root, "go.mod")+":/src/go.mod:z,ro",
				"--workdir=/src",
				"docker.io/golang:alpine",
				"go",
				"mod",
				"download",
				"-x",
			)
		},
		true,
	)
}
