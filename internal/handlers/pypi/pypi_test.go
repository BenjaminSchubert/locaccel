package pypi

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestHandlesNotModified(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("ETag", "1234")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer upstream.Close()

	logger := testutils.TestLogger(t, nil)
	handler := &http.ServeMux{}
	RegisterHandler(
		upstream.URL+"/simple",
		upstream.URL+"/cdn",
		handler,
		testutils.NewClient(t, false, logger),
		nil,
	)
	proxy, _ := testutils.NewServer(t, handler, "proxy", "proxy", logger)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		proxy.URL+"/simple/package-1",
		nil,
	)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotModified, resp.StatusCode)
	assert.Empty(t, string(data))
	assert.Equal(t, []string{"1234"}, resp.Header.Values("ETag"))
	require.NoError(t, resp.Body.Close())
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
