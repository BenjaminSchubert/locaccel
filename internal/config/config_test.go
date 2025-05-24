package config_test

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/units"
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
cache:
  path: ./cache
  private: true
  quota_low: 1
  quota_high: 10
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
			Host: "0.0.0.0",
			Cache: config.Cache{
				"./cache",
				true,
				units.NewDiskQuotaInBytes(units.Bytes{Bytes: 1}),
				units.NewDiskQuotaInBytes(units.Bytes{Bytes: 10}),
			},
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
