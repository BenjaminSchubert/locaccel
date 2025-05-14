package config

import "os"

type OciRegistry struct {
	Remote string
	Port   uint16
}

type Config struct {
	Host            string
	CachePath       string
	AdminInterface  string
	EnableProfiling bool
	OciRegistries   []OciRegistry
}

func New() *Config {
	conf := Config{
		Host:           "localhost",
		CachePath:      "_cache",
		AdminInterface: "localhost:3130",
		OciRegistries: []OciRegistry{
			{"https://registry-1.docker.io", 3131},
			{"https://ghcr.io", 3132},
			{"https://quay.io", 3133},
		},
	}

	if os.Getenv("LOCACCEL_ENABLE_PROFILING") == "1" {
		conf.EnableProfiling = true
	}

	return &conf
}
