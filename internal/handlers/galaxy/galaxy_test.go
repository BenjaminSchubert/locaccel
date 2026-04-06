package galaxy

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/benjaminschubert/locaccel/internal/handlers/proxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallGalaxyPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"galaxy",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			RegisterHandler(
				"https://galaxy.ansible.com",
				handler,
				client,
				upstreamCaches,
			)
		},
		func(t *testing.T, serverURL string) {
			t.Helper()

			logger := testutils.TestLogger(t)
			handler := &http.ServeMux{}
			proxy.RegisterHandler(
				[]string{"deb.debian.org"},
				handler,
				testutils.NewClient(t, false, logger),
				nil,
			)
			prox, _ := testutils.NewServer(
				t,
				handler,
				"proxy",
				"proxy",
				testutils.NewRequestCounterMiddleware(t),
				logger,
			)

			testutils.Execute(
				t,
				"podman",
				"run",
				"--rm",
				"--interactive",
				"--network=host",
				"--dns=127.0.0.127",
				"--env=http_proxy="+prox.URL,
				"--env=no_proxy=localhost,127.0.0.1",
				"--env=ANSIBLE_GALAXY_SERVER="+serverURL,
				"debian:stable-slim",
				"bash",
				"-c",
				"apt-get update && apt-get install --assume-yes --no-install-recommends ansible-core && ansible-galaxy collection install -v community.general",
			)
		},
		false,
		0,
		1,
	)
}
