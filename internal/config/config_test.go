package config_test

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/config"
)

func TestCanParseValidConfiguration(t *testing.T) {
	t.Parallel()

	configFile := path.Join(t.TempDir(), "config.yml")

	require.NoError(
		t,
		os.WriteFile(
			configFile,
			[]byte(`
host: 0.0.0.0
cache: ./cache
admin_interface: localhost:8192
profiling: true
log:
  level: error
  format: console
oci_registries:
  - upstream: https://registry-1.docker.io
    port: 1234
pypi_registries:
  - upstream: https://pypi.org
    cdn: https://files.pythonhosted.org
    port: 1235`),
			0o600,
		),
	)

	conf, err := config.Parse(configFile)
	require.NoError(t, err)
	require.Equal(
		t,
		&config.Config{
			Host:            "0.0.0.0",
			CachePath:       "./cache",
			AdminInterface:  "localhost:8192",
			EnableProfiling: true,
			Log:             config.Log{"error", "console"},
			OciRegistries: []config.OciRegistry{
				{Upstream: "https://registry-1.docker.io", Port: 1234},
			},
			PyPIRegistries: []config.PyPIRegistry{
				{Upstream: "https://pypi.org", CDN: "https://files.pythonhosted.org", Port: 1235},
			},
		},
		conf,
	)
}
