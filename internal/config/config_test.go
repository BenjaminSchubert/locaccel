package config_test

import (
	"os"
	"path"
	"testing"

	"github.com/rs/zerolog"
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
log_level: error
oci_registries:
  - remote: https://registry-1.docker.io
    port: 1234`),
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
			LogLevel:        zerolog.LevelErrorValue,
			OciRegistries: []config.OciRegistry{
				{Remote: "https://registry-1.docker.io", Port: 1234},
			},
		},
		conf,
	)
}
