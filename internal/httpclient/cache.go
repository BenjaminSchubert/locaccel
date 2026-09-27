package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"
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

type CacheList map[string]map[string]CachedResponses

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
	cache.stopWait.Add(1)
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

	err = c.db.Iterate(
		ctx,
		func(key []byte, responses *database.Entry[CachedResponses]) error {
			uri, err := url.Parse(string(key))
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

func (c *Cache) New(key []byte, value CachedResponses) error {
	return c.db.New(key, value)
}

func (c *Cache) Save(key []byte, entry *database.Entry[CachedResponses]) error {
	return c.db.Save(key, entry)
}

func (c *Cache) Get(key []byte, entry *database.Entry[CachedResponses]) error {
	return c.db.Get(key, entry)
}

func (c *Cache) Open(hash string, logger *zerolog.Logger) (io.ReadCloser, error) {
	return c.cache.Open(hash, logger)
}

func (c *Cache) SetupIngestion(
	src io.ReadCloser,
	onIngest func(hash string),
	onCleanup func(),
	logger *zerolog.Logger,
) io.ReadCloser {
	return c.cache.SetupIngestion(src, onIngest, onCleanup, logger)
}

func (c *Cache) List(ctx context.Context, hostname, logId string) (CacheList, error) {
	list := make(CacheList)

	err := c.db.Iterate(
		ctx,
		func(key []byte, responses *database.Entry[CachedResponses]) error {
			k := string(key)
			uri, err := url.Parse(k)
			if err != nil {
				return err
			}

			if uri.Hostname() != hostname {
				return nil
			}

			method, path, _ := strings.Cut(k, "+")
			if list[path] == nil {
				list[path] = make(map[string]CachedResponses, 1)
			}
			list[path][method] = responses.Value
			return nil
		},
		logId,
	)
	return list, err
}

func (c *Cache) Remove(key []byte, logger *zerolog.Logger) error {
	entry := new(database.Entry[CachedResponses])
	if err := c.db.Get(key, entry); err != nil {
		return err
	}
	if err := c.db.Delete(key, entry); err != nil {
		return err
	}

	var err error
	for _, resp := range entry.Value {
		if fErr := c.cache.Delete(resp.ContentHash, logger); fErr != nil {
			if errors.Is(fErr, fs.ErrNotExist) {
				logger.Warn().Str("file", resp.ContentHash).Msg("Hash was not found in cache")
				continue
			}
			err = fErr
		}
	}

	return err
}

func (c *Cache) CleanupOldEntries(logId string) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	logger := c.logger.With().Str("id", logId).Logger()
	filecacheLogger := logger.With().Str("component", "filecache").Logger()

	// Prune files from the file cache
	_, existingHashes, err := c.cache.Prune(&filecacheLogger)
	if err != nil {
		if !errors.Is(err, filecache.ErrGCleanupNotRequired) {
			logger.Error().Err(err).Msg("an error happened trying to reclaim space")
		}
	}

	referenced := make(map[string]bool, len(existingHashes))
	for hash := range existingHashes {
		referenced[hash] = false
	}

	// Remove old entries from the database
	err = c.removeUnusedDatabaseEntries(referenced, logId)
	if err != nil {
		logger.Error().Err(err).Msg("unable to list all files in the cache during cleanup")
	}

	logger.Info().Msg("File cache cleaned up, vacuuming database")
	if err := c.db.RunGarbageCollector(); err != nil && !errors.Is(err, database.ErrNoRewrite) {
		logger.Error().Err(err).Msg("an error happened trying to vacuum the database")
		return
	}

	for hash, inUse := range referenced {
		if !inUse {
			logger.Debug().Str("hash", hash).Msg("Deleting unreferenced file")
			if err := c.cache.Delete(hash, &logger); err != nil {
				logger.Error().Err(err).Str("hash", hash).Msg("Error removing unreferenced file")
			}
		}
	}
}

func (c *Cache) removeUnusedDatabaseEntries(referenced map[string]bool, logId string) error {
	knownMissing := make(map[string]struct{}, 10)

	return c.db.Iterate(
		context.Background(),
		func(key []byte, value *database.Entry[CachedResponses]) error {
			return c.pruneDatabaseEntry(key, value, referenced, knownMissing)
		},
		logId,
	)
}

func (c *Cache) pruneDatabaseEntry(
	key []byte,
	value *database.Entry[CachedResponses],
	referenced map[string]bool,
	missing map[string]struct{},
) error {
	validValues := make(CachedResponses, 0)
	for _, resp := range value.Value {
		if _, ok := referenced[resp.ContentHash]; ok {
			referenced[resp.ContentHash] = true
			validValues = append(validValues, resp)
			continue
		}

		if _, ok := missing[resp.ContentHash]; !ok {
			_, err := c.cache.Stat(resp.ContentHash)
			switch {
			case err == nil:
				referenced[resp.ContentHash] = true
				validValues = append(validValues, resp)
			case errors.Is(err, fs.ErrNotExist):
				missing[resp.ContentHash] = struct{}{}
			default:
				return fmt.Errorf(
					"unable to check existence for file %s: %w",
					resp.ContentHash,
					err,
				)
			}
		}
	}

	if len(validValues) == len(value.Value) {
		// Files actually exist, not pruning
		return nil
	}

	if len(validValues) == 0 {
		// ErrConflict will get reconciled on next cleanup, ignore
		if err := c.db.Delete(key, value); !errors.Is(err, database.ErrConflict) {
			return err
		}
		return nil
	}

	value.Value = validValues
	// ErrConflict will get reconciled on next cleanup, ignore
	if err := c.db.Save(key, value); !errors.Is(err, database.ErrConflict) {
		return err
	}
	return nil
}

func (c *Cache) ManageCache() {
	defer c.stopWait.Done()
	ticker := time.NewTicker(15 * time.Minute)
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
