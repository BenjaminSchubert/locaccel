package httpclient

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/filecache"
)

type CacheStatistics struct {
	DatabaseSize     int64
	DatabaseEntries  int64
	FileCacheSize    int64
	FileCacheEntries int64
	UsagePerHostName map[string]struct{ Entries, Size int64 }
}

type Cache struct {
	db     *database.Database[CachedResponses, *CachedResponses]
	cache  *filecache.FileCache
	logger *zerolog.Logger
}

func NewCache(cachePath string, logger *zerolog.Logger) (*Cache, error) {
	cache, err := filecache.NewFileCache(path.Join(cachePath, "cache"))
	if err != nil {
		return nil, fmt.Errorf("unable to initialize file cache: %w", err)
	}

	// Ensure the db logger is not too chatty
	dbLogger := logger.With().Str("component", "database").Logger()
	if dbLogger.GetLevel() < zerolog.WarnLevel {
		dbLogger = dbLogger.Level(zerolog.WarnLevel)
	}

	db, err := database.NewDatabase[CachedResponses](
		path.Join(cachePath, "db"),
		&dbLogger,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize database: %w", err)
	}

	cacheLogger := logger.With().Str("component", "cache").Logger()
	return &Cache{db, cache, &cacheLogger}, nil
}

func (c *Cache) Close() error {
	return c.db.Close()
}

func (c *Cache) GetStatistics(ctx context.Context, logId string) (CacheStatistics, error) {
	dbEntries, dbTotalSize, err := c.db.GetStatistics()
	if err != nil {
		return CacheStatistics{}, err
	}

	fileCacheEntries, fileCacheTotalSize, err := c.cache.GetStatistics()
	if err != nil {
		return CacheStatistics{}, err
	}

	usagePerHostname := map[string]struct{ Entries, Size int64 }{}

	err = c.db.Iterate(ctx,
		func(key string, responses CachedResponses) error {
			uri, err := url.Parse(key)
			if err != nil {
				return err
			}

			hostname := uri.Hostname()

			entry := usagePerHostname[hostname]
			entry.Entries += int64(len(responses))

			for _, resp := range responses {
				size, err := c.cache.GetSize(resp.ContentHash, c.logger)
				if err != nil {
					return err
				}
				entry.Size += size
			}
			usagePerHostname[hostname] = entry

			return nil
		},
		logId,
	)
	if err != nil {
		return CacheStatistics{}, err
	}

	return CacheStatistics{
		dbTotalSize, dbEntries, fileCacheTotalSize, fileCacheEntries, usagePerHostname,
	}, nil
}
