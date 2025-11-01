package filecache_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/iotest"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var errTest = errors.New("testerror")

const testData = "1234567890"

func ingest(t *testing.T, cache *filecache.FileCache, content string, logger *zerolog.Logger) {
	t.Helper()

	buf := bytes.NewBufferString(content)
	reader := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, logger)
	_, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
}

func TestCanIngestAndRecover(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	computedHash := ""

	reader := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(testData)),
		func(hash string) { computedHash = hash },
		logger,
	)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	err = reader.Close()
	require.NoError(t, err)
	assert.Equal(
		t,
		"d12e417e04494572b561ba2c12c3d7f9e5107c4747e27b9a8a54f8480c63e841",
		computedHash,
	)

	fp, err := cache.Open(computedHash, logger)
	require.NoError(t, err)

	result, err := io.ReadAll(fp)
	require.NoError(t, err)

	assert.Equal(t, []byte("1234567890"), result)
}

func TestHandlesConcurrentWrites(t *testing.T) {
	t.Parallel()
	logger := testutils.TestLogger(t)

	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	hash1 := ""
	hash2 := ""

	reader1 := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(testData)),
		func(hash string) { hash1 = hash },
		logger,
	)
	reader2 := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(testData)),
		func(hash string) { hash2 = hash },
		logger,
	)

	data, err := io.ReadAll(reader1)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	data, err = io.ReadAll(reader2)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	require.NoError(t, reader1.Close())
	require.NoError(t, reader2.Close())

	assert.Equal(t, hash1, hash2)
}

func TestHandlesErrorsWhileWriting(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	logger := testutils.TestLogger(t)

	cache, err := filecache.NewFileCache(cacheDir, 100, 1000, logger)
	require.NoError(t, err)

	reader := cache.SetupIngestion(
		io.NopCloser(iotest.ErrReader(errTest)),
		func(hash string) { assert.Fail(t, "Hash should not have been called") },
		logger,
	)

	_, err = io.ReadAll(reader)
	require.ErrorIs(t, err, errTest)

	require.NoError(t, reader.Close())
}

func TestDoesNotIngestFilesThatAreTooBig(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	logger := testutils.TestLogger(t)

	cache, err := filecache.NewFileCache(cacheDir, 5, 10, logger)
	require.NoError(t, err)

	reader := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString("toolong")),
		func(hash string) { assert.Fail(t, "Hash should not have been called") },
		logger,
	)

	_, err = io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
}

func TestReturnsErrorOpeningNonExistentFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	fp, err := cache.Open("nonexistent", logger)
	require.ErrorIs(t, err, filecache.ErrCannotOpen)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Nil(t, fp)
}

func TestCanStatFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	var hash string

	buf := bytes.NewBufferString("hello world!")
	reader := cache.SetupIngestion(io.NopCloser(buf), func(h string) { hash = h }, logger)
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	stat, err := cache.Stat(hash)
	require.NoError(t, err)
	require.Equal(t, int64(len(output)), stat.Size())
}

func TestCanGetStatistics(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	count, totalSize, err := cache.GetStatistics()
	assert.Equal(t, int64(0), count)
	assert.Equal(t, units.Bytes{Bytes: 0}, totalSize)
	require.NoError(t, err)

	for _, content := range []string{"one", "two", "three", "four", "five"} {
		ingest(t, cache, content, logger)
	}

	// tmp should be ignored
	buf := bytes.NewBufferString("six")
	reader := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, logger)
	_, err = io.ReadAll(reader)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })

	count, totalSize, err = cache.GetStatistics()
	assert.Equal(t, int64(5), count)
	assert.Equal(t, units.Bytes{Bytes: 19}, totalSize)
	require.NoError(t, err)
}

func TestCanRemoveOldFiles(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 10, 20, logger)
	require.NoError(t, err)

	// Under the limit
	for _, content := range []string{"first", "second", "third"} {
		ingest(t, cache, content, logger)
	}

	count, err := cache.Prune(logger)
	require.ErrorIs(t, err, filecache.ErrGCleanupNotRequired)
	require.Equal(t, int64(0), count)

	// Now we need cleaning
	ingest(t, cache, "fourth", logger)

	count, err = cache.Prune(logger)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestCanGetAllHashes(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	count, totalSize, err := cache.GetStatistics()
	assert.Equal(t, int64(0), count)
	assert.Equal(t, units.Bytes{Bytes: 0}, totalSize)
	require.NoError(t, err)

	for _, content := range []string{"one", "two"} {
		ingest(t, cache, content, logger)
	}

	// tmp should be ignored
	buf := bytes.NewBufferString("three")
	reader := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, logger)
	_, err = io.ReadAll(reader)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })

	hashes, err := cache.GetAllHashes()
	require.NoError(t, err)
	assert.ElementsMatch(
		t,
		[]string{
			"d33fb48ab5adff269ae172b29a6913ff04f6f266207a7a8e976f2ecd571d4492",
			"dc770fff53f50835f8cc957e01c0d5731d3c2ed544c375493a28c09be5e09763",
		},
		hashes,
	)
}

func BenchmarkFileIngestion(b *testing.B) {
	for _, size := range []string{"10KiB", "100KiB", "1MiB", "10MiB", "100MiB", "1GiB"} {
		b.Run(size, func(b *testing.B) {
			sizeB, err := units.DecodeBytes(size)
			require.NoError(b, err)

			logger := testutils.TestLogger(b)
			cache, err := filecache.NewFileCache(b.TempDir(), sizeB.Bytes, sizeB.Bytes*10, logger)
			require.NoError(b, err)

			data := make([]byte, sizeB.Bytes)
			n, err := rand.Read(data)
			require.NoError(b, err)
			require.Equal(b, sizeB.Bytes, int64(n))

			buf := bytes.NewReader(data)

			b.ResetTimer()
			for b.Loop() {
				r := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, logger)
				n, err := io.Copy(io.Discard, r)
				require.NoError(b, err)
				require.Equal(b, sizeB.Bytes, n)
				require.NoError(b, r.Close())
				buf.Reset(data)
			}
		})
	}
}
