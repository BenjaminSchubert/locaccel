package oci_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/oci"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/middleware"
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

func preparePodmanIsolation(t *testing.T, workdir, serverURL, registryName string) []string {
	t.Helper()

	require.NoError(t, os.MkdirAll(workdir, 0o750))

	registriesConf := path.Join(workdir, "registries.conf")
	storageConf := path.Join(workdir, "storage.conf")
	dataPath := path.Join(workdir, "data")

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
		map[string]string{"registry": registryName, "location": uri.Host},
	)

	// And the storage configuration
	writeTemplate(
		t,
		"podman-storage.conf",
		`[storage]
rootless_storage_path="{{.}}"`,
		storageConf,
		dataPath,
	)

	t.Cleanup(func() {
		require.NoError(
			t,
			exec.Command("podman", "unshare", "rm", "-rf", dataPath).Run(), //nolint:gosec
		)
	})

	return []string{
		"CONTAINERS_REGISTRIES_CONF=" + registriesConf,
		"CONTAINERS_STORAGE_CONF=" + storageConf,
	}
}

func TestDownloadImageWithPodman(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	for _, testcase := range []struct {
		registry string
		location string
		image    string
	}{
		{"docker.io", "https://registry-1.docker.io", "docker.io/alpine"},
		{"gcr.io", "https://gcr.io", "gcr.io/distroless/static"},
		{"quay.io", "https://quay.io", "quay.io/navidys/prometheus-podman-exporter"},
	} {
		t.Run(testcase.registry, func(t *testing.T) {
			t.Parallel()

			logger := testutils.TestLogger(t)

			handler := &http.ServeMux{}
			oci.RegisterHandler(testcase.location, handler, testutils.NewClient(t, logger))
			server := httptest.NewServer(middleware.ApplyAllMiddlewares(handler, logger))
			defer server.Close()

			// Generate the registry configuration
			env := preparePodmanIsolation(
				t, path.Join(t.TempDir(), "podman"), server.URL, testcase.registry)

			cmd := exec.Command("podman", "pull", testcase.image) //nolint:gosec
			cmd.Env = append(cmd.Env, env...)
			output, err := cmd.CombinedOutput()
			require.NoErrorf(t, err, "Running podman failed:\n%s", output)
		})
	}
}
