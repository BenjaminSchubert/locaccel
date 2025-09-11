package config

import (
	"net/url"
	"os"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/units"
)

type GoProxy struct {
	Upstream       string
	Port           uint16
	UpstreamCaches []*url.URL `yaml:"upstream_caches"`
}
type NpmRegistry struct {
	Upstream       string
	Scheme         string
	Port           uint16
	UpstreamCaches []*url.URL `yaml:"upstream_caches"`
}

type OciRegistry struct {
	Upstream       string
	Port           uint16
	UpstreamCaches []*url.URL `yaml:"upstream_caches"`
}

type PyPIRegistry struct {
	Upstream       string
	CDN            string
	Port           uint16
	UpstreamCaches []*url.URL `yaml:"upstream_caches"`
}

type Proxy struct {
	AllowedUpstreams []string `yaml:"allowed_upstreams"`
	Port             uint16
	UpstreamCaches   []*url.URL `yaml:"upstream_caches"`
}

type Log struct {
	Level  string
	Format string
}

type Cache struct {
	Path      string
	Private   bool
	QuotaLow  units.DiskQuota `yaml:"quota_low"`
	QuotaHigh units.DiskQuota `yaml:"quota_high"`
}

func (c Cache) GetQuotaLow() (units.Bytes, error) {
	err := os.MkdirAll(c.Path, 0o750)
	if err != nil {
		return units.Bytes{}, err
	}
	return c.QuotaLow.Bytes(c.Path)
}

func (c Cache) GetQuotaHigh() (units.Bytes, error) {
	err := os.MkdirAll(c.Path, 0o750)
	if err != nil {
		return units.Bytes{}, err
	}
	return c.QuotaHigh.Bytes(c.Path)
}

type Config struct {
	Host            string
	Cache           Cache
	AdminInterface  string `yaml:"admin_interface"`
	EnableMetrics   bool   `yaml:"metrics"`
	EnableProfiling bool   `yaml:"profiling"`
	Log             Log
	GoProxies       []GoProxy      `yaml:"go_proxies"`
	NpmRegistries   []NpmRegistry  `yaml:"npm_registries"`
	OciRegistries   []OciRegistry  `yaml:"oci_registries"`
	PyPIRegistries  []PyPIRegistry `yaml:"pypi_registries"`
	Proxies         []Proxy
}

func getBaseConfig() *Config {
	return &Config{
		Host: "localhost",
		Cache: Cache{
			"_cache/",
			false,
			units.NewDiskQuotaInPercent(10),
			units.NewDiskQuotaInPercent(20),
		},
		AdminInterface: "localhost:3130",
		EnableMetrics:  true,
		Log:            Log{zerolog.LevelInfoValue, "json"},
	}
}

func Parse(configPath string) (*Config, error) {
	c := getBaseConfig()

	fp, err := os.Open(configPath) //nolint:gosec
	if err != nil {
		return c, err
	}

	decoder := yaml.NewDecoder(fp)
	decoder.KnownFields(true)
	err = decoder.Decode(&c)

	applyOverrides(c)
	return c, err
}

func Default() *Config {
	conf := getBaseConfig()
	conf.GoProxies = []GoProxy{
		{"https://proxy.golang.org", 3136, nil},
	}
	conf.OciRegistries = []OciRegistry{
		{"https://registry-1.docker.io", 3131, nil},
		{"https://gcr.io", 3132, nil},
		{"https://quay.io", 3133, nil},
	}
	conf.NpmRegistries = []NpmRegistry{
		{"https://registry.npmjs.org/", "http", 3135, nil},
	}
	conf.PyPIRegistries = []PyPIRegistry{
		{"https://pypi.org/", "https://files.pythonhosted.org", 3134, nil},
	}
	conf.Proxies = []Proxy{{
		[]string{
			// Debian
			"deb.debian.org",
			// Ubuntu
			"archive.ubuntu.com", "security.ubuntu.com",
		},
		3142,
		nil,
	}}

	applyOverrides(conf)
	return conf
}

func applyOverrides(conf *Config) {
	if os.Getenv("LOCACCEL_ENABLE_PROFILING") == "1" {
		conf.EnableProfiling = true
	}

	if val, ok := os.LookupEnv("LOCACCEL_LOG_LEVEL"); ok {
		conf.Log.Level = val
	}

	if val, ok := os.LookupEnv("LOCACCEL_LOG_FORMAT"); ok {
		conf.Log.Format = val
	}

	if val, ok := os.LookupEnv("LOCACCEL_CACHE_PATH"); ok {
		conf.Cache.Path = val
	}

	if val, ok := os.LookupEnv("LOCACCEL_HOST"); ok {
		conf.Host = val
	}

	if val, ok := os.LookupEnv("LOCACCEL_ADMIN_INTERFACE"); ok {
		conf.AdminInterface = val
	}
}
