package pypi

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

var pytestInfo []byte = nil

func TestInstallPythonPackages(t *testing.T) {
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

			testutils.Execute(
				t,
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
		},
		true,
	)
}

func BenchmarkJSONRewrite(b *testing.B) {
	if pytestInfo == nil {
		req, err := http.NewRequestWithContext(
			b.Context(),
			"GET",
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

	for b.Loop() {
		d := bytes.Clone(pytestInfo)
		_, err := rewriteJsonV1(d, cdn, encodedCDN)
		require.NoError(b, err)
	}
}
