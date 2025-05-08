package config

type Config struct {
	Address   string
	CachePath string
}

func New() (Config, error) {
	config := Config{
		Address:   "localhost:16080",
		CachePath: "_cache/",
	}

	return config, nil
}
