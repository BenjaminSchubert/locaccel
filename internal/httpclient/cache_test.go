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
)

func validateCache(
	t *testing.T,
	cache *Cache,
	expected map[string]CachedResponses,
	startTime time.Time,
) {
	t.Helper()

	expectedHashes := make([]string, 0, len(expected))
	for _, responses := range expected {
		for _, resp := range responses {
			expectedHashes = append(expectedHashes, resp.ContentHash)
		}
	}

	entriesInDB := make(map[string]CachedResponses, len(expected))
	err := cache.db.Iterate(
		t.Context(),
		func(key string, value *database.Entry[CachedResponses]) error {
			for i := range value.Value {
				assert.Greater(t, value.Value[i].TimeAtRequestCreated, startTime)
				assert.Greater(
					t,
					value.Value[i].TimeAtResponseReceived,
					value.Value[i].TimeAtRequestCreated,
				)

				value.Value[i].TimeAtRequestCreated = time.Time{}
				value.Value[i].TimeAtResponseReceived = time.Time{}
			}

			entriesInDB[key] = value.Value
			return nil
		},
		"test",
	)
	require.NoError(t, err)

	hashes, err := cache.cache.GetAllHashes()
	require.NoError(t, err)

	assert.Equal(t, expected, entriesInDB)
	assert.ElementsMatch(t, expectedHashes, hashes)
}

func ingest(t *testing.T, cache *Cache, content string) string {
	t.Helper()

	var hash string

	reader := cache.cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(content)),
		func(h string) { hash = h },
		cache.logger,
	)
	_, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return hash
}

func addEntry(t *testing.T, cache *Cache, key string, data []string) {
	t.Helper()

	responses := make(CachedResponses, len(data))
	for i, d := range data {
		responses[i].TimeAtRequestCreated = time.Now()
		responses[i].TimeAtResponseReceived = time.Now().Add(time.Second)
		responses[i].StatusCode = http.StatusOK
		responses[i].ContentHash = ingest(t, cache, d)
	}

	require.NoError(t, cache.db.New(key, responses))
}

func TestCanGetStatisticsOnEmptyCache(t *testing.T) {
	t.Parallel()

	cache, err := NewCache(t.TempDir(), 10, 20, testutils.TestLogger(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	stats, err := cache.GetStatistics(t.Context(), "test")
	require.NoError(t, err)
	require.Equal(t, CacheStatistics{0, 0, 0, 0, map[string]struct {
		Entries int64
		Size    int64
	}{}}, stats)
}

func TestCanGetStatistics(t *testing.T) {
	t.Parallel()

	cache, err := NewCache(t.TempDir(), 10, 20, testutils.TestLogger(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	addEntry(t, cache, "https://one.test/hello", []string{"one"})
	addEntry(
		t,
		cache,
		"https://two.test/hello",
		[]string{"two", "two-two"},
	)
	addEntry(
		t,
		cache,
		"https://three.test/hello",
		[]string{"three"},
	)
	addEntry(
		t,
		cache,
		"https://three.test/hi",
		[]string{"three-two"},
	)

	stats, err := cache.GetStatistics(t.Context(), "test")
	require.NoError(t, err)
	require.Equal(t, CacheStatistics{603, 4, 27, 5, map[string]struct {
		Entries int64
		Size    int64
	}{"one.test": {1, 3}, "two.test": {2, 10}, "three.test": {2, 14}}}, stats)
}

func TestDoesNotCleanOldEntriesWithCacheUnderLimit(t *testing.T) {
	t.Parallel()

	startTime := time.Now()

	cache, err := NewCache(t.TempDir(), 10, 20, testutils.TestLogger(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	// Empty cache
	cache.CleanupOldEntries("test")

	addEntry(t, cache, "https://test.test", []string{"helloworld"})

	// Under the limit
	cache.CleanupOldEntries("test")

	validateCache(t, cache, map[string]CachedResponses{"https://test.test": {
		{
			"7bb205244d808356318ec65d0ae54f32ee3a7bab5dfaf431b01e567e03baab4f",
			http.StatusOK,
			http.Header{},
			http.Header{},
			time.Time{},
			time.Time{},
		},
	}}, startTime)
}

func TestCanCleanOldEntries(t *testing.T) {
	t.Parallel()

	cachePath := t.TempDir()
	startTime := time.Now()

	cache, err := NewCache(cachePath, 10, 20, testutils.TestLogger(t))
	require.NoError(t, err)
	defer func() { require.NoError(t, cache.Close()) }()

	addEntry(t, cache, "https://one.test", []string{"one"})
	addEntry(t, cache, "https://two.test", []string{"two-one", "two-two"})
	addEntry(t, cache, "https://three.test", []string{"three"})

	require.NoError(
		t,
		os.Chtimes(
			// "two-one"
			path.Join(
				cachePath,
				"cache/60/e077fe1f739faa929a30a4bb4440eb6d82cb2776e87252a5d533af247897e2",
			),
			time.Time{},
			time.Now().Add(-time.Hour),
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
			time.Time{},
			time.Now().Add(-time.Minute),
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
					time.Time{},
					time.Time{},
				},
			},
			"https://two.test": {
				{
					"37a541978486c4df6b74665c1328fa7ae1d997ecf242635cfaacc34e48c4e0c1",
					http.StatusOK,
					http.Header{},
					http.Header{},
					time.Time{},
					time.Time{},
				},
			},
		},
		startTime,
	)
}
