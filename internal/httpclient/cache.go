package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/rs/xid"
	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/units"
)

type CacheStatistics struct {
	DatabaseSize     units.Bytes
	DatabaseEntries  int64
	FileCacheSize    units.Bytes
	FileCacheEntries int64
	UsagePerHostName map[string]struct {
		Entries int64
		Size    units.Bytes
	}
}

type Cache struct {
	db         *database.Database[CachedResponses, *CachedResponses]
	cache      *filecache.FileCache
	logger     *zerolog.Logger
	stopSignal chan struct{}
	stopWait   *sync.WaitGroup
	cacheLock  *sync.Mutex
}

func NewCache(
	cachePath string,
	quotaLow, quotaHigh units.Bytes,
	logger *zerolog.Logger,
) (*Cache, error) {
	fileCacheLogger := logger.With().Str("component", "filecache").Logger()
	fileCache, err := filecache.NewFileCache(
		path.Join(cachePath, "cache"),
		quotaLow.Bytes,
		quotaHigh.Bytes,
		&fileCacheLogger,
	)
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

	cache := Cache{db, fileCache, logger, make(chan struct{}), &sync.WaitGroup{}, &sync.Mutex{}}
	go cache.ManageCache()
	return &cache, nil
}

func (c *Cache) Close() error {
	if c.stopSignal == nil {
		// Already stopped
		return nil
	}

	close(c.stopSignal)
	c.stopWait.Wait()
	err := c.db.Close()
	c.stopSignal = nil
	return err
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

	usagePerHostname := map[string]struct {
		Entries int64
		Size    units.Bytes
	}{}

	err = c.db.Iterate(ctx,
		func(key string, responses *database.Entry[CachedResponses]) error {
			uri, err := url.Parse(key)
			if err != nil {
				return err
			}

			hostname := uri.Hostname()

			entry := usagePerHostname[hostname]
			entry.Entries += int64(len(responses.Value))

			for _, resp := range responses.Value {
				stat, err := c.cache.Stat(resp.ContentHash)
				if err == nil {
					entry.Size.Bytes += stat.Size()
				} else if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
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

func (c *Cache) CleanupOldEntries(logId string) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	logger := c.logger.With().Str("id", logId).Logger()
	filecacheLogger := logger.With().Str("component", "filecache").Logger()

	// Prune files from the file cache
	_, err := c.cache.Prune(&filecacheLogger)
	if err != nil {
		if !errors.Is(err, filecache.ErrGCleanupNotRequired) {
			logger.Error().Err(err).Msg("an error happened trying to reclaim space")
		}
		return
	}

	// Get all the files left
	hashesList, err := c.cache.GetAllHashes()
	if err != nil {
		logger.Error().Err(err).Msg("unable to list all files in the cache during cleanup")
	}

	hashes := make(map[string]bool, len(hashesList))
	for _, hash := range hashesList {
		hashes[hash] = false
	}

	// Remove old entries from the database
	err = c.removeUnusedDatabaseEntries(hashes, logId)
	if err != nil {
		logger.Error().Err(err).Msg("unable to list all files in the cache during cleanup")
	}

	logger.Info().Msg("File cache cleaned up, vacuuming database")
	if err := c.db.RunGarbageCollector(); err != nil && !errors.Is(err, database.ErrNoRewrite) {
		logger.Error().Err(err).Msg("an error happened trying to vacuum the database")
		return
	}
}

func (c *Cache) removeUnusedDatabaseEntries(knownHashes map[string]bool, logId string) error {
	return c.db.Iterate(
		context.Background(),
		func(key string, value *database.Entry[CachedResponses]) error {
			hasMissing := false

			for _, resp := range value.Value {
				if _, ok := knownHashes[resp.ContentHash]; ok {
					knownHashes[resp.ContentHash] = true
					continue
				}
				hasMissing = true
			}

			if hasMissing {
				return c.pruneDatabaseEntry(key, value)
			}
			return nil
		},
		logId,
	)
}

func (c *Cache) pruneDatabaseEntry(key string, value *database.Entry[CachedResponses]) error {
	validValues := make(CachedResponses, 0)
	for _, resp := range value.Value {
		_, err := c.cache.Stat(resp.ContentHash)
		if err == nil {
			validValues = append(validValues, resp)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("unable to check existence for file %s: %w", resp.ContentHash, err)
		}
	}

	if len(validValues) == len(value.Value) {
		// Files actually exist, not pruning
		return nil
	}

	if len(validValues) == 0 {
		return c.db.Delete(key, value)
	}

	value.Value = validValues
	return c.db.Save(key, value)
}

func (c *Cache) ManageCache() {
	c.stopWait.Add(1)
	defer c.stopWait.Done()
	ticker := time.NewTimer(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.CleanupOldEntries(xid.New().String())
		case <-c.stopSignal:
			return
		}
	}
}
