package rubygem_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/benjaminschubert/locaccel/internal/handlers/rubygem"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallRubyGemPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"rubygem",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			rubygem.RegisterHandler(
				"https://rubygems.org",
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
				"docker.io/ruby:slim",
				"gem",
				"install",
				"--source="+serverURL,
				"multi_xml",
			)
		},
		true,
	)
}
