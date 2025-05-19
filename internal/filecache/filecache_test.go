package filecache_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

var errTest = errors.New("testerror")

const testData = "1234567890"

func TestCanIngestAndRecover(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir())
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

	cache, err := filecache.NewFileCache(t.TempDir())
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

	cache, err := filecache.NewFileCache(cacheDir)
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

func TestReturnsErrorOpeningNonExistentFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir())
	require.NoError(t, err)

	fp, err := cache.Open("nonexistent", logger)
	require.ErrorIs(t, err, filecache.ErrCannotOpen)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Nil(t, fp)
}

func TestCanGetSizeOfFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir())
	require.NoError(t, err)

	var hash string

	buf := bytes.NewBufferString("hello world!")
	reader := cache.SetupIngestion(io.NopCloser(buf), func(h string) { hash = h }, logger)
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	size, err := cache.GetSize(hash, logger)
	require.NoError(t, err)
	require.Equal(t, int64(len(output)), size)
}

func TestCanGetStatistics(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)
	cache, err := filecache.NewFileCache(t.TempDir())
	require.NoError(t, err)

	count, totalSize, err := cache.GetStatistics()
	assert.Equal(t, int64(0), count)
	assert.Equal(t, int64(0), totalSize)
	require.NoError(t, err)

	for _, content := range []string{"one", "two", "three", "four", "five"} {
		buf := bytes.NewBufferString(content)
		reader := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, logger)
		_, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
	}

	count, totalSize, err = cache.GetStatistics()
	assert.Equal(t, int64(5), count)
	assert.Equal(t, int64(19), totalSize)
	require.NoError(t, err)
}
