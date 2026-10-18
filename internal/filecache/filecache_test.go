package filecache_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/iotest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var errTest = errors.New("testerror")

const testData = "1234567890"

func ingest(
	t *testing.T,
	cache *filecache.FileCache,
	content string,
	logger *zerolog.Logger,
) string {
	t.Helper()

	var hash string

	notify := make(chan struct{}, 1)
	buf := bytes.NewBufferString(content)
	reader := cache.SetupIngestion(
		testutils.ReadCloserNotifier{Reader: buf, Notify: notify},
		func(h string) { hash = h },
		func() {},
		logger,
	)
	_, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	<-notify
	return hash
}

func TestCanIngestAndRecover(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	ingested := make(chan string, 1)

	reader := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString(testData)),
		func(hash string) { ingested <- hash },
		func() {},
		logger,
	)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	computedHash := <-ingested
	require.NoError(t, reader.Close())
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
	logger := testutils.TestLogger(t, nil)

	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	notify1 := make(chan struct{}, 1)
	notify2 := make(chan struct{}, 1)

	hash1 := ""
	hash2 := ""

	reader1 := cache.SetupIngestion(
		testutils.ReadCloserNotifier{Reader: bytes.NewBufferString(testData), Notify: notify1},
		func(hash string) { hash1 = hash },
		func() {},
		logger,
	)
	reader2 := cache.SetupIngestion(
		testutils.ReadCloserNotifier{Reader: bytes.NewBufferString(testData), Notify: notify2},
		func(hash string) { hash2 = hash },
		func() {},
		logger,
	)

	data, err := io.ReadAll(reader1)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	data, err = io.ReadAll(reader2)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	<-notify1
	<-notify2

	require.NoError(t, reader1.Close())
	require.NoError(t, reader2.Close())

	assert.Equal(t, hash1, hash2)
}

func TestHandlesErrorsWhileWriting(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	logger := testutils.TestLogger(t, []string{"an error happened ingesting the file"})

	cache, err := filecache.NewFileCache(cacheDir, 100, 1000, logger)
	require.NoError(t, err)

	notify := make(chan struct{}, 1)
	reader := cache.SetupIngestion(
		testutils.ReadCloserNotifier{Reader: iotest.ErrReader(errTest), Notify: notify},
		func(hash string) { assert.Fail(t, "Hash should not have been called") },
		func() {},
		logger,
	)

	_, err = io.ReadAll(reader)
	require.ErrorIs(t, err, errTest)

	require.NoError(t, reader.Close())
	<-notify // Ensure it's finalized so we catch the error on logs
}

func TestDoesNotIngestFilesThatAreTooBig(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	logger := testutils.TestLogger(t, nil)

	cache, err := filecache.NewFileCache(cacheDir, 5, 10, logger)
	require.NoError(t, err)

	reader := cache.SetupIngestion(
		io.NopCloser(bytes.NewBufferString("toolong")),
		func(hash string) { assert.Fail(t, "Hash should not have been called") },
		func() {},
		logger,
	)

	_, err = io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
}

func TestReturnsErrorOpeningNonExistentFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	fp, err := cache.Open("nonexistent", logger)
	require.ErrorIs(t, err, filecache.ErrCannotOpen)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Nil(t, fp)
}

func TestCanStatFile(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	ingested := make(chan string, 1)

	buf := bytes.NewBufferString("hello world!")
	reader := cache.SetupIngestion(
		io.NopCloser(buf),
		func(h string) { ingested <- h },
		func() {},
		logger,
	)
	output, err := io.ReadAll(reader)
	require.NoError(t, err)

	hash := <-ingested
	require.NoError(t, reader.Close())

	stat, err := cache.Stat(hash)
	require.NoError(t, err)
	require.Equal(t, int64(len(output)), stat.Size())
}

func TestCanGetStatistics(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
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
	r := testutils.NewSlowReader([]byte("six"), 10*time.Millisecond)
	reader := cache.SetupIngestion(r, func(string) {}, func() {}, logger)
	t.Cleanup(func() {
		require.NoError(t, r.Close())
		_, err = io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		cache.WaitUntilNoJobsInFlight()
	})

	count, totalSize, err = cache.GetStatistics()
	assert.Equal(t, int64(5), count)
	assert.Equal(t, units.Bytes{Bytes: 19}, totalSize)
	require.NoError(t, err)
}

func TestCanRemoveOldFiles(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
	cache, err := filecache.NewFileCache(t.TempDir(), 10, 20, logger)
	require.NoError(t, err)

	// Under the limit
	for _, content := range []string{"first", "second", "third"} {
		ingest(t, cache, content, logger)
	}

	count, allHashes, err := cache.Prune(logger)
	require.ErrorIs(t, err, filecache.ErrGCleanupNotRequired)
	assert.Equal(t, map[string]struct{}{
		"22896bcbc3d1c76a0b90c4c3523dbea532ad63196fafdbd52cced52200d3dae4": {},
		"42f1d0a285aebbec81c29b9e334aaa322f6f24ac7d5f14c3b89aa50a9bc7b2d1": {},
		"cd85637651ec7a557bddd61c5ddd1df21ad8bbbaf6c3c098482b3ed1c1014964": {},
	}, allHashes)
	assert.Equal(t, int64(0), count)

	// Now we need cleaning
	ingest(t, cache, "fourth", logger)

	count, allHashes, err = cache.Prune(logger)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Equal(
		t,
		map[string]struct{}{"b20f46117e4ff694c5c5c655af80d57f35b21e4e9a213f92df4a08eaca0a3a30": {}},
		allHashes,
	)
}

func TestCanGetAllHashes(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
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
	r := testutils.NewSlowReader([]byte("three"), 30*time.Millisecond)
	reader := cache.SetupIngestion(r, func(string) {}, func() {}, logger)
	t.Cleanup(func() {
		cache.WaitUntilNoJobsInFlight()
		require.NoError(t, r.Close())
		_, err = io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		cache.WaitUntilNoJobsInFlight()
	})

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

func TestCanDelete(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t, nil)
	cache, err := filecache.NewFileCache(t.TempDir(), 100, 1000, logger)
	require.NoError(t, err)

	hash := ingest(t, cache, "one", logger)

	err = cache.Delete(hash, logger)
	require.NoError(t, err)
}

func BenchmarkFileIngestion(b *testing.B) {
	for _, size := range []string{"10KiB", "100KiB", "1MiB", "10MiB", "100MiB", "1GiB"} {
		b.Run(size, func(b *testing.B) {
			sizeB, err := units.DecodeBytes(size)
			require.NoError(b, err)

			logger := testutils.TestLogger(b, nil)
			cache, err := filecache.NewFileCache(b.TempDir(), sizeB.Bytes, sizeB.Bytes*10, logger)
			require.NoError(b, err)

			data := make([]byte, sizeB.Bytes)
			n, err := rand.Read(data)
			require.NoError(b, err)
			require.Equal(b, sizeB.Bytes, int64(n))

			buf := bytes.NewReader(data)

			b.ResetTimer()
			for b.Loop() {
				r := cache.SetupIngestion(io.NopCloser(buf), func(string) {}, func() {}, logger)
				n, err := io.Copy(io.Discard, r)
				require.NoError(b, err)
				require.Equal(b, sizeB.Bytes, n)
				require.NoError(b, r.Close())
				buf.Reset(data)
			}
		})
	}
}
