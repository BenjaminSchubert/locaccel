package oci_test

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/oci"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func writeTemplate(t *testing.T, name, templateString, destination string, data any) {
	t.Helper()

	tmpl, err := template.New(name).Parse(templateString)
	require.NoError(t, err)

	file, err := os.Create(destination) //nolint:gosec
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	require.NoError(t, tmpl.Execute(file, data))
}

func TestDownloadImageWithPodman(t *testing.T) {
	t.Parallel()

	for _, testcase := range []struct {
		registry string
		location string
		image    string
	}{
		{"docker.io", "https://registry-1.docker.io", "docker.io/alpine"},
		{"gcr.io", "https://gcr.io", "gcr.io/distroless/static"},
		{"quay.io", "https://quay.io", "quay.io/navidys/prometheus-podman-exporter"},
		{"ghcr.io", "https://ghcr.io", "ghcr.io/benjaminschubert/locaccel"},
	} {
		t.Run(testcase.registry, func(t *testing.T) {
			t.Parallel()

			testutils.RunIntegrationTestsForHandler(
				t,
				"oci",
				func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
					oci.RegisterHandler(testcase.location, handler, client, upstreamCaches)
				},
				func(t *testing.T, serverURL string) {
					t.Helper()

					registriesConf := path.Join(t.TempDir(), "registries.conf")
					uri, err := url.Parse(serverURL)
					require.NoError(t, err)
					writeTemplate(
						t,
						"podman-registries.conf",
						`[[registry]]
				prefix="{{.registry}}"
				location="{{.location}}"
				insecure = true`,
						registriesConf,
						map[string]string{"registry": testcase.registry, "location": uri.Host},
					)

					testutils.Execute(
						t,
						"podman",
						"run",
						"--rm",
						"--interactive",
						"--network=host",
						"--volume="+registriesConf+":/etc/containers/registries.conf:z,ro",
						"quay.io/podman/stable",
						"podman",
						"--debug",
						"pull",
						testcase.image,
					)
				},
				false,
			)
		})
	}
}
