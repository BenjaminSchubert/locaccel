package goproxy_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/goproxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
)

func TestInstallGoPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	for _, useUpstreamCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("upstreamCache=%v", useUpstreamCache), func(t *testing.T) {
			t.Parallel()

			logger := testutils.TestLogger(t)
			var upstreams []*url.URL
			counterMiddleware := testutils.NewRequestCounterMiddleware(t)

			if useUpstreamCache {
				upstreamLogger := logger.With().Str("type", "upstream").Logger()
				handler := &http.ServeMux{}
				goproxy.RegisterHandler(
					"https://proxy.golang.org",
					handler,
					testutils.NewClient(t, &upstreamLogger),
					nil,
				)
				server := testutils.NewServer(
					t,
					handler,
					"go",
					"upstream",
					counterMiddleware,
					&upstreamLogger,
				)
				upstream, err := url.Parse(server.URL)
				require.NoError(t, err)
				upstreams = append(upstreams, upstream)
			}

			localLogger := logger.With().Str("type", "local").Logger()
			handler := &http.ServeMux{}
			goproxy.RegisterHandler(
				"https://proxy.golang.org",
				handler,
				testutils.NewClient(t, &localLogger),
				upstreams,
			)
			server := testutils.NewServer(
				t,
				handler,
				"go",
				"local",
				counterMiddleware,
				&localLogger,
			)

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
				"--volume="+path.Join(root, "go.mod")+":/src/go.mod:z,ro",
				"--volume="+path.Join(root, "go.sum")+":/src/go.sum:z,ro",
				"--workdir=/src",
				"docker.io/golang:alpine",
				"go",
				"get",
				"./...",
			)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}
