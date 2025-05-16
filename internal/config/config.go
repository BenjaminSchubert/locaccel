package config

import (
	"os"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

type OciRegistry struct {
	Upstream string
	Port     uint16
}

type PyPIRegistry struct {
	Upstream string
	CDN      string
	Port     uint16
}

type Proxy struct {
	AllowedUpstreams []string `yaml:"allowed_upstreams"`
	Port             uint16
}

type Log struct {
	Level  string
	Format string
}

type Config struct {
	Host            string
	CachePath       string `yaml:"cache"`
	AdminInterface  string `yaml:"admin_interface"`
	EnableProfiling bool   `yaml:"profiling"`
	Log             Log
	OciRegistries   []OciRegistry  `yaml:"oci_registries"`
	PyPIRegistries  []PyPIRegistry `yaml:"pypi_registries"`
	Proxies         []Proxy
}

func getBaseConfig() *Config {
	return &Config{
		Host:           "localhost",
		CachePath:      "_cache/",
		AdminInterface: "localhost:3130",
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
	conf.OciRegistries = []OciRegistry{
		{"https://registry-1.docker.io", 3131},
		{"https://ghcr.io", 3132},
		{"https://quay.io", 3133},
	}
	conf.PyPIRegistries = []PyPIRegistry{
		{"https://pypi.org/", "https://files.pythonhosted.org", 3134},
	}
	conf.Proxies = []Proxy{{
		[]string{
			// Debian
			"deb.debian.org",
			// Ubuntu
			"archive.ubuntu.com", "security.ubuntu.com",
		},
		3142,
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
}
