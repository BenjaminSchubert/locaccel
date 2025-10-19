package config_test

import (
	"io/fs"
	"net/url"
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
metrics: false
profiling: true
log:
  level: error
  format: console
oci_registries:
  - upstream: https://registry-1.docker.io
    port: 1234
    upstream_caches: [https://upstream:1234]
pypi_registries:
  - upstream: https://pypi.org
    cdn: https://files.pythonhosted.org
    port: 1235`),
			0o600,
		),
	)

	conf, err := config.Parse(configFile, func(s string) (string, bool) { return "", false })
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
			EnableMetrics:   false,
			EnableProfiling: true,
			Log:             config.Log{"error", "console"},
			OciRegistries: []config.OciRegistry{
				{
					Upstream: "https://registry-1.docker.io",
					Port:     1234,
					UpstreamCaches: []config.SerializableURL{
						{&url.URL{Scheme: "https", Host: "upstream:1234"}},
					},
				},
			},
			PyPIRegistries: []config.PyPIRegistry{
				{Upstream: "https://pypi.org", CDN: "https://files.pythonhosted.org", Port: 1235},
			},
		},
		conf,
	)
}

func TestCanGetQuotaLow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	conf := config.Default(func(s string) (string, bool) { return "", false })
	conf.Cache.Path = dir
	conf.Cache.QuotaLow = units.NewDiskQuotaInBytes(units.Bytes{Bytes: 100})

	quota, err := conf.Cache.GetQuotaLow()
	require.NoError(t, err)
	require.Equal(t, units.Bytes{Bytes: 100}, quota)
}

func TestCanGetQuotaHigh(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	conf := config.Default(func(s string) (string, bool) { return "", false })
	conf.Cache.Path = dir
	conf.Cache.QuotaHigh = units.NewDiskQuotaInBytes(units.Bytes{Bytes: 100})

	quota, err := conf.Cache.GetQuotaHigh()
	require.NoError(t, err)
	require.Equal(t, units.Bytes{Bytes: 100}, quota)
}

func TestGetQuotaReportsProblemOnCachePathInvalid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500)) //nolint:gosec

	conf := config.Default(func(s string) (string, bool) { return "", false })
	conf.Cache.Path = dir + "/cache"

	quota, err := conf.Cache.GetQuotaHigh()
	require.ErrorIs(t, err, fs.ErrPermission)
	require.Equal(t, units.Bytes{}, quota)
}

func TestReportsCannotReadConfig(t *testing.T) {
	t.Parallel()

	_, err := config.Parse("nonexistent", func(s string) (string, bool) { return "", false })
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestCanSetOverridesViaEnvironment(t *testing.T) {
	t.Parallel()

	conf := config.Default(func(envvar string) (string, bool) {
		switch envvar {
		case "LOCACCEL_ENABLE_PROFILING":
			return "1", true
		case "LOCACCEL_LOG_LEVEL":
			return "debug", true
		case "LOCACCEL_LOG_FORMAT":
			return "console", true
		case "LOCACCEL_CACHE_PATH":
			return "cache", true
		case "LOCACCEL_HOST":
			return "0.0.0.0", true
		case "LOCACCEL_ADMIN_INTERFACE":
			return "0.0.0.0:1000", true
		default:
			return "", false
		}
	})

	require.Equal(
		t,
		&config.Config{
			Host: "0.0.0.0",
			Cache: config.Cache{
				Path:      "cache",
				Private:   false,
				QuotaLow:  units.NewDiskQuotaInPercent(10),
				QuotaHigh: units.NewDiskQuotaInPercent(20),
			},
			AdminInterface:  "0.0.0.0:1000",
			EnableMetrics:   true,
			EnableProfiling: true,
			Log:             config.Log{Level: "debug", Format: "console"},
			GoProxies: []config.GoProxy{
				{
					Upstream: "https://proxy.golang.org",
					SumDBURL: "https://sum.golang.org/",
					Port:     3143,
				},
			},
			NpmRegistries: []config.NpmRegistry{
				{Upstream: "https://registry.npmjs.org/", Scheme: "http", Port: 3144},
			},
			OciRegistries: []config.OciRegistry{
				{Upstream: "https://registry-1.docker.io", Port: 3131},
				{Upstream: "https://gcr.io", Port: 3132},
				{Upstream: "https://quay.io", Port: 3133},
				{Upstream: "https://ghcr.io", Port: 3134},
			},
			PyPIRegistries: []config.PyPIRegistry{
				{Upstream: "https://pypi.org/", CDN: "https://files.pythonhosted.org", Port: 3145},
			},
			Proxies: []config.Proxy{
				{
					AllowedUpstreams: []string{
						"deb.debian.org",
						"archive.ubuntu.com",
						"security.ubuntu.com",
					},
					Port: 3142,
				},
			},
			RubyGemRegistries: []config.RubyGemRegistry{
				{Upstream: "https://rubygems.org", Port: 3146},
			},
		},
		conf,
	)
}
