package httpclient

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

func validateCache(
	t *testing.T,
	cache *Cache,
	expectedResponses map[string]CachedResponses,
	expectedHashes []string,
) {
	t.Helper()

	if expectedHashes == nil {
		expectedHashesMap := map[string]struct{}{}
		for _, responses := range expectedResponses {
			for _, resp := range responses {
				expectedHashesMap[resp.ContentHash] = struct{}{}
			}
		}
		expectedHashes = make([]string, 0, len(expectedHashesMap))
		for hash := range expectedHashesMap {
			expectedHashes = append(expectedHashes, hash)
		}
	}

	entriesInDB := make(map[string]CachedResponses, len(expectedResponses))
	err := cache.db.Iterate(
		t.Context(),
		func(key []byte, value *database.Entry[CachedResponses]) error {
			entriesInDB[string(key)] = value.Value
			return nil
		},
		"test",
	)
	require.NoError(t, err)

	hashes, err := cache.cache.GetAllHashes()
	require.NoError(t, err)

	assert.Equal(
		t,
		expectedResponses,
		entriesInDB,
		"Entries in the database are not what is expected",
	)
	assert.ElementsMatch(t, expectedHashes, hashes)
}

func ingest(t *testing.T, cache *Cache, content string) string {
	t.Helper()

	var hash string

	reader := cache.cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(content)),
		func(h string) { hash = h },
		func() {},
		cache.logger,
	)
	_, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return hash
}

func addEntry(t *testing.T, cache *Cache, key string, data []string, clock *Clock) {
	t.Helper()

	responses := make(CachedResponses, len(data))
	for i, d := range data {
		responses[i].TimeAtResponseCreation = clock.Now().Local()
		clock.Advance()
		responses[i].StatusCode = http.StatusOK
		responses[i].ContentHash = ingest(t, cache, d)
	}

	require.NoError(t, cache.db.New([]byte(key), responses))
}

func TestCanGetStatisticsOnEmptyCache(t *testing.T) {
	t.Parallel()

	cache, err := NewCache(
		t.TempDir(),
		units.Bytes{Bytes: 10},
		units.Bytes{Bytes: 20},
		testutils.TestLogger(t),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	stats, err := cache.GetStatistics(t.Context(), "test")
	require.NoError(t, err)
	require.Equal(t, CacheStatistics{units.Bytes{}, 0, units.Bytes{}, 0, map[string]struct {
		Entries int64
		Size    units.Bytes
	}{}}, stats)
}

func TestCanGetStatistics(t *testing.T) {
	t.Parallel()

	clock := &Clock{}

	cache, err := NewCache(
		t.TempDir(),
		units.Bytes{Bytes: 10},
		units.Bytes{Bytes: 20},
		testutils.TestLogger(t),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	addEntry(t, cache, "https://one.test/hello", []string{"one"}, clock)
	addEntry(
		t,
		cache,
		"https://two.test/hello",
		[]string{"two", "two-two"},
		clock,
	)
	addEntry(
		t,
		cache,
		"https://three.test/hello",
		[]string{"three"},
		clock,
	)
	addEntry(
		t,
		cache,
		"https://three.test/hi",
		[]string{"three-two"},
		clock,
	)

	stats, err := cache.GetStatistics(t.Context(), "test")
	require.NoError(t, err)
	require.Equal(
		t,
		CacheStatistics{units.Bytes{Bytes: 528}, 4, units.Bytes{Bytes: 27}, 5, map[string]struct {
			Entries int64
			Size    units.Bytes
		}{"one.test": {1, units.Bytes{Bytes: 3}}, "two.test": {2, units.Bytes{Bytes: 10}}, "three.test": {2, units.Bytes{Bytes: 14}}}},
		stats,
	)
}

func TestDoesNotCleanOldEntriesWithCacheUnderLimit(t *testing.T) {
	t.Parallel()

	clock := &Clock{}

	cache, err := NewCache(
		t.TempDir(),
		units.Bytes{Bytes: 10},
		units.Bytes{Bytes: 20},
		testutils.TestLogger(t),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	// Empty cache
	cache.CleanupOldEntries("test")

	addEntry(t, cache, "https://test.test", []string{"helloworld"}, clock)

	// Under the limit
	cache.CleanupOldEntries("test")

	validateCache(t, cache, map[string]CachedResponses{"https://test.test": {
		{
			"7bb205244d808356318ec65d0ae54f32ee3a7bab5dfaf431b01e567e03baab4f",
			http.StatusOK,
			http.Header{},
			http.Header{},
			clock.Now().Add(-time.Second).Local(),
		},
	}}, nil)
}

func TestCanCleanOldEntries(t *testing.T) {
	t.Parallel()

	clock := &Clock{time.Now()}
	cachePath := t.TempDir()

	cache, err := NewCache(
		cachePath,
		units.Bytes{Bytes: 10},
		units.Bytes{Bytes: 20},
		testutils.TestLogger(t),
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	addEntry(t, cache, "https://one.test", []string{"one"}, clock)
	addEntry(t, cache, "https://two.test", []string{"two-one", "two-two"}, clock)
	addEntry(t, cache, "https://three.test", []string{"three"}, clock)

	require.NoError(
		t,
		os.Chtimes(
			// "two-one"
			path.Join(
				cachePath,
				"cache/60/e077fe1f739faa929a30a4bb4440eb6d82cb2776e87252a5d533af247897e2",
			),
			clock.Now(),
			clock.Now().Add(-time.Minute),
		),
	)
	require.NoError(
		t,
		os.Chtimes(
			// "three"
			path.Join(
				cachePath,
				"cache/db/ec0e689fb63bd729147727129d854e9d590ab620a18bbbcb3317d268d6fd72",
			),
			clock.Now(),
			clock.Now().Add(-time.Minute),
		),
	)

	cache.CleanupOldEntries("test")

	validateCache(
		t,
		cache,
		map[string]CachedResponses{
			"https://one.test": {
				{
					"d33fb48ab5adff269ae172b29a6913ff04f6f266207a7a8e976f2ecd571d4492",
					http.StatusOK,
					http.Header{},
					http.Header{},
					clock.Now().Add(-4 * time.Second).Local(),
				},
			},
			"https://two.test": {
				{
					"37a541978486c4df6b74665c1328fa7ae1d997ecf242635cfaacc34e48c4e0c1",
					http.StatusOK,
					http.Header{},
					http.Header{},
					clock.Now().Add(-2 * time.Second).Local(),
				},
			},
		},
		nil,
	)
}
