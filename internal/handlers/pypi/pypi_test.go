package pypi

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

var pytestInfo []byte = nil

func TestInstallPythonPackages(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		command []string
		image   string
	}{
		{[]string{"pip"}, "docker.io/python:slim"},
		{[]string{"uv", "pip"}, "docker.io/astral/uv:python3.11-trixie-slim"},
	} {
		t.Run(strings.Join(tc.command, " "), func(t *testing.T) {
			t.Parallel()

			testutils.RunIntegrationTestsForHandler(
				t,
				"pypi",
				func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
					RegisterHandler(
						"https://pypi.org",
						"https://files.pythonhosted.org",
						handler,
						client,
						upstreamCaches,
					)
				},
				func(t *testing.T, serverURL string) {
					t.Helper()

					cmd := []string{
						"run",
						"--rm",
						"--interactive",
						"--network=host",
						"--dns=127.0.0.127",
						"--env=PIP_INDEX_URL=" + serverURL + "/simple",
						"--env=UV_DEFAULT_INDEX=" + serverURL + "/simple",
						"--env=PIP_DISABLE_PIP_VERSION_CHECK=true",
						"--env=UV_SYSTEM_PYTHON=1",
						tc.image,
					}
					cmd = append(cmd, tc.command...)
					cmd = append(cmd, "install", "colorama")

					testutils.Execute(t, "podman", cmd...)
				},
				false,
				0,
				0,
				nil,
			)
		})
	}
}

func BenchmarkJSONRewrite(b *testing.B) {
	if pytestInfo == nil {
		req, err := http.NewRequestWithContext(
			b.Context(),
			http.MethodGet,
			"https://pypi.org/simple/pytest/",
			nil,
		)
		require.NoError(b, err)
		req.Header.Add("Accept", "application/vnd.pypi.simple.v1+json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(b, err)

		pytestInfo, err = io.ReadAll(resp.Body)
		require.NoError(b, err)
		require.NoError(b, resp.Body.Close())
	}

	cdn := "https://files.pythonhosted.org"
	encodedCDN := "/cnd/" + base64.StdEncoding.EncodeToString([]byte(cdn))

	jsonHandler := handlers.NewJSONHandler()

	for b.Loop() {
		err := rewriteJsonV1(pytestInfo, cdn, encodedCDN, jsonHandler)
		require.NoError(b, err)
		jsonHandler.Buffer.Reset()
	}
}
