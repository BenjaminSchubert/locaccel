package config

import (
	"os"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

type OciRegistry struct {
	Remote string
	Port   uint16
}

type Config struct {
	Host            string
	CachePath       string        `yaml:"cache"`
	AdminInterface  string        `yaml:"admin_interface"`
	EnableProfiling bool          `yaml:"profiling"`
	LogLevel        string        `yaml:"log_level"`
	OciRegistries   []OciRegistry `yaml:"oci_registries"`
}

func getBaseConfig() *Config {
	return &Config{
		Host:           "localhost",
		CachePath:      "_cache/",
		AdminInterface: "localhost:3130",
		LogLevel:       zerolog.LevelInfoValue,
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

	return c, err
}

func Default() *Config {
	conf := getBaseConfig()
	conf.OciRegistries = []OciRegistry{
		{"https://registry-1.docker.io", 3131},
		{"https://ghcr.io", 3132},
		{"https://quay.io", 3133},
	}

	if os.Getenv("LOCACCEL_ENABLE_PROFILING") == "1" {
		conf.EnableProfiling = true
	}

	return conf
}
