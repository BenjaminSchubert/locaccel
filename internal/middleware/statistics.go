package middleware

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/types/atomic"
)

type Statistics struct {
	CacheHits       atomic.Uint64
	CacheMisses     atomic.Uint64
	UnCacheable     atomic.Uint64
	Revalidated     atomic.Uint64
	BytesServed     atomic.Uint64
	BytesDownloaded atomic.Uint64
}

func LoadSavedStatistics(path string, logger *zerolog.Logger) (*Statistics, error) {
	stats := new(Statistics)

	fp, err := os.Open(path) //nolint:gosec
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return stats, nil
		}
		logger.Debug().Msg("Statistics don't exist. Creating new one")
		return nil, err
	}
	defer func() {
		if err := fp.Close(); err != nil {
			logger.Error().Err(err).Msg("Error closing the statistics file")
		}
	}()

	decoder := json.NewDecoder(fp)
	if err := decoder.Decode(stats); err != nil {
		return nil, err
	}

	logger.Debug().Str("path", path).Msg("Statistics loaded from disk")
	return stats, nil
}

func (s *Statistics) Save(path string, logger *zerolog.Logger) error {
	fp, err := os.Create(path) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if err := fp.Close(); err != nil {
			logger.Error().Err(err).Msg("Error closing the statistics file")
		}
	}()

	encoder := json.NewEncoder(fp)
	return encoder.Encode(s)
}
