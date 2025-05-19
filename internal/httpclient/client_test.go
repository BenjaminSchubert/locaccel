package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/logging"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func getAllEntriesInDB(
	t *testing.T,
	cacheRoot string,
	logger *zerolog.Logger,
) map[string]CachedResponses {
	t.Helper()

	db, err := badger.Open(
		badger.DefaultOptions(path.Join(cacheRoot, "db")).
			WithLogger(logging.NewLoggerAdapter(logger)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })

	cachedResponses := map[string]CachedResponses{}

	err = db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			entry := CachedResponses{}
			_, err = entry.UnmarshalMsg(val)
			if err != nil {
				return err
			}

			cachedResponses[string(item.Key())] = entry
		}

		return nil
	})
	require.NoError(t, err)

	return cachedResponses
}

func getAllEntriesInFileCache(t *testing.T, cacheRoot string) []string {
	t.Helper()

	cachePath := path.Join(cacheRoot, "cache")
	files, err := filepath.Glob(path.Join(cachePath, "*", "*"))
	require.NoError(t, err)

	filePaths := []string{}
	for _, file := range files {
		filePaths = append(filePaths, strings.TrimPrefix(file, cachePath+"/"))
	}

	return filePaths
}

func validateCache(
	t *testing.T,
	cacheRoot string,
	logger *zerolog.Logger,
	expected map[string]CachedResponses,
	startTime time.Time,
) {
	t.Helper()

	entriesInDB := getAllEntriesInDB(t, cacheRoot, logger)
	for _, entries := range entriesInDB {
		for i := range entries {
			assert.Greater(t, entries[i].TimeAtRequestCreated, startTime)
			assert.Greater(t, entries[i].TimeAtResponseReceived, entries[i].TimeAtRequestCreated)

			entries[i].TimeAtRequestCreated = time.Time{}
			entries[i].TimeAtResponseReceived = time.Time{}
		}
	}
	assert.Equal(t, expected, entriesInDB)

	expectedFiles := make([]string, 0, len(expected))
	for _, responses := range expected {
		for _, resp := range responses {
			expectedFiles = append(expectedFiles, resp.ContentHash[:2]+"/"+resp.ContentHash[2:])
		}
	}

	assert.Equal(t, expectedFiles, getAllEntriesInFileCache(t, cacheRoot))
}

func setup(t *testing.T) (client *Client, cache *Cache, valCache func(map[string]CachedResponses)) {
	t.Helper()

	cachePath := t.TempDir()
	logger := testutils.TestLogger(t)

	cache, err := NewCache(cachePath, logger)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, cache.Close()) })

	currentTime := time.Now()

	return New(&http.Client{}, cache, logger), cache, func(expected map[string]CachedResponses) {
		validateCache(t, cachePath, logger, expected, currentTime)
	}
}

func makeRequest(
	t *testing.T,
	client *Client,
	method, url string,
	headers http.Header,
) (resp *http.Response, body string) {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	logger := testutils.TestLogger(t)
	req = req.WithContext(logger.WithContext(req.Context()))

	req.Header = headers

	resp, err = client.Do(req)
	require.NoError(t, err)

	bodyB, err := io.ReadAll(resp.Body)
	assert.NoError(t, resp.Body.Close())
	require.NoError(t, err)

	return resp, string(bodyB)
}

func TestClientForwardsNonCacheableMethods(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, err := w.Write([]byte("hello!"))
			assert.NoError(t, err)
		}
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodPost, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "hello!", body)

	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{})
}

func TestClientDoesNotCachedErrors(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.CloseClientConnections()
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = client.Do(req) //nolint:bodyclose
	require.ErrorContains(t, err, "EOF")
	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{})
}

func TestClientDoesNotCacheUncacheableResponses(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{})
}

func TestClientCachesCacheableResponses(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "public")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date":           resp.Header["Date"],
				},
				http.Header{},
				time.Time{},
				time.Time{},
			},
		},
	})
}

func TestClientReturnsResponseFromCacheWhenPossible(t *testing.T) {
	t.Parallel()

	client, _, _ := setup(t)

	wasCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.False(t, wasCalled, "The service did not serve the request from cache")
		w.Header().Add("Cache-Control", "public, max-age=20")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
		wasCalled = true
	}))
	t.Cleanup(srv.Close)

	// Initial Query
	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose

	date := resp.Header["Date"]

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Cache-Control":  []string{"public, max-age=20"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
		},
		resp.Header,
	)

	// Second Query
	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"0"},
			"Cache-Control":  []string{"public, max-age=20"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
		},
		resp.Header,
	)
}

func TestClientRespectsVaryHeadersAndCachesAll(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count += 1
		w.Header().Add("Cache-Control", "public, max-age=30")
		w.Header().Add("Vary", "Count")
		_, err := w.Write(fmt.Appendf(nil, "Hello %s!", r.Header.Get("Count")))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	makeQuery := func(count int, date []string) *http.Response {
		resp, body := makeRequest(
			t,
			client,
			http.MethodGet,
			srv.URL,
			http.Header{"Count": []string{strconv.Itoa(count)}},
		)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, fmt.Sprintf("Hello %d!", count), body)

		expectedHeader := http.Header{
			"Cache-Control":  []string{"public, max-age=30"},
			"Content-Length": []string{"8"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
			"Vary":           []string{"Count"},
		}
		if date == nil {
			expectedHeader["Date"] = resp.Header["Date"]
		} else {
			assert.NotNil(t, resp.Header["Age"])
			expectedHeader["Age"] = resp.Header["Age"]
		}

		assert.Equal(t, expectedHeader, resp.Header)

		return resp
	}

	// Initial Query
	resp1 := makeQuery(1, nil) //nolint:bodyclose

	// Second Query, should not be cached
	resp2 := makeQuery(2, nil) //nolint:bodyclose

	// First query again should hit the cache
	makeQuery(1, resp1.Header["Date"]) //nolint:bodyclose

	// Second query again should hit the cache
	makeQuery(2, resp2.Header["Date"]) //nolint:bodyclose

	require.Equal(t, 2, count, "The API was hit %d times instead of 2", count)

	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"9dea94da2f7eb6112119b81792afb3bc0f18d19d0b6d5cc1ca3a51ebeef7b670",
				200,
				http.Header{
					"Cache-Control":  []string{"public, max-age=30"},
					"Content-Length": []string{"8"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date":           resp1.Header["Date"],
					"Vary":           []string{"Count"},
				},
				http.Header{"Count": []string{"1"}},
				time.Time{},
				time.Time{},
			},
			{
				"bab02792998098aa075831b5c79424be14f4d50f316cf555d4d54250258dda6a",
				200,
				http.Header{
					"Cache-Control":  []string{"public, max-age=30"},
					"Content-Length": []string{"8"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date":           resp1.Header["Date"],
					"Vary":           []string{"Count"},
				},
				http.Header{"Count": []string{"2"}},
				time.Time{},
				time.Time{},
			},
		},
	})
}

func TestValidationEtag(t *testing.T) {
	t.Parallel()

	client, cache, validateCache := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "public, no-cache")
		w.Header().Add("Etag", "Hello")

		if slices.ContainsFunc(
			r.Header["If-None-Match"],
			func(e string) bool { return e == "Hello" },
		) {
			w.Header().Add("Stale", "1")
			w.WriteHeader(http.StatusNotModified)
			return
		}

		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	// First request should get the answer
	resp1, body := makeRequest(t, client, http.MethodGet, srv.URL, http.Header{}) //nolint:bodyclose
	assert.Equal(t, 200, resp1.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Cache-Control":  []string{"public, no-cache"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           resp1.Header["Date"],
			"Etag":           []string{"Hello"},
		},
		resp1.Header,
	)

	// Second request should revalidate
	resp2, body := makeRequest(t, client, http.MethodGet, srv.URL, http.Header{}) //nolint:bodyclose
	assert.Equal(t, 200, resp2.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"0"},
			"Cache-Control":  []string{"public, no-cache"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           resp2.Header["Date"],
			"Etag":           []string{"Hello"},
			"Stale":          []string{"1"},
		},
		resp2.Header,
	)

	require.NoError(t, cache.Close())

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public, no-cache"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date":           resp2.Header["Date"],
					"Etag":           []string{"Hello"},
					"Stale":          []string{"1"},
				},
				http.Header{},
				time.Time{},
				time.Time{},
			},
		},
	})
}

func TestClientReturnsResponseFromCacheIfDisconnected(t *testing.T) {
	t.Parallel()

	client, _, _ := setup(t)

	wasCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wasCalled {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}

		w.Header().Add("Cache-Control", "public, max-age=0")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
		wasCalled = true
	}))
	t.Cleanup(srv.Close)

	// Initial Query
	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose

	date := resp.Header["Date"]

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
		},
		resp.Header,
	)

	// Second query getting a 5XX, should be served by the cache
	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"0"},
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
		},
		resp.Header,
	)

	// Third Query, should still be served by the cache
	srv.Close()

	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"0"},
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           date,
		},
		resp.Header,
	)
}
