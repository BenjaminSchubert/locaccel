package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

type Clock struct {
	current time.Time
}

func (c *Clock) Since(t time.Time) time.Duration {
	return c.current.Sub(t)
}

func (c *Clock) Now() time.Time {
	return c.current
}

func (c *Clock) Advance() {
	c.current = c.current.Add(time.Second)
}

func setup(
	t *testing.T,
) (client *Client, clock *Clock, valCache func(map[string]CachedResponses), valQueries func([]string)) {
	t.Helper()

	cachePath := t.TempDir()
	logger := testutils.TestLogger(t)
	testTime, err := time.Parse(time.RFC3339, "2024-01-01T12:13:14Z")
	require.NoError(t, err)
	clock = &Clock{testTime}

	cache, err := NewCache(cachePath, units.Bytes{Bytes: 100}, units.Bytes{Bytes: 1000}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, cache.Close()) })

	hits := []string{}

	return New(
			&http.Client{Transport: &http.Transport{}},
			cache,
			logger,
			false,
			func(r *http.Request, status string) { hits = append(hits, status) },
			clock.Now,
			clock.Since,
		),
		clock,
		func(expected map[string]CachedResponses) {
			validateCache(t, cache, expected)
		},
		func(expected []string) { assert.Equal(t, expected, hits) }
}

func makeRequest(
	t *testing.T,
	client *Client,
	method, uri string,
	headers http.Header,
	upstreamCaches []*url.URL,
) (resp *http.Response, body string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, uri, nil)
	require.NoError(t, err)
	logger := testutils.TestLogger(t)
	req = req.WithContext(logger.WithContext(req.Context()))

	req.Header = headers

	resp, err = client.Do(req, UpstreamCache{upstreamCaches, false})
	require.NoError(t, err)

	bodyB, err := io.ReadAll(resp.Body)
	assert.NoError(t, resp.Body.Close())
	require.NoError(t, err)

	return resp, string(bodyB)
}

func TestClientForwardsNonCacheableMethods(t *testing.T) {
	t.Parallel()

	client, _, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, err := w.Write([]byte("hello!"))
			assert.NoError(t, err)
		}
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodPost, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "hello!", body)

	validateCache(map[string]CachedResponses{})
	validateQueries([]string{"miss"})
}

func TestClientDoesNotCachedErrors(t *testing.T) {
	t.Parallel()

	client, _, validateCache, validateQueries := setup(t)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.CloseClientConnections()
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = client.Do(req, UpstreamCache{}) //nolint:bodyclose
	require.ErrorContains(t, err, "EOF")

	validateCache(map[string]CachedResponses{})
	validateQueries([]string{})
}

func TestClientDoesNotCacheUncacheableResponses(t *testing.T) {
	t.Parallel()

	client, _, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-store")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	validateCache(map[string]CachedResponses{})
	validateQueries([]string{"miss"})
}

func TestClientCachesCacheableResponses(t *testing.T) {
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "public")
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
				},
				http.Header{},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss"})
}

func TestClientReturnsResponseFromCacheWhenPossible(t *testing.T) {
	t.Parallel()

	client, clock, _, validateQueries := setup(t)

	wasCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.False(t, wasCalled, "The service did not serve the request from cache")
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Cache-Control", "public, max-age=20")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
		wasCalled = true
	}))
	t.Cleanup(srv.Close)

	// Initial Query
	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Cache-Control":  []string{"public, max-age=20"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
		},
		resp.Header,
	)

	// Second Query
	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"1"},
			"Cache-Control":  []string{"public, max-age=20"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
		},
		resp.Header,
	)

	validateQueries([]string{"miss", "hit"})
}

func TestClientReturnsResponseFromCacheForLastModified(t *testing.T) {
	t.Parallel()

	client, clock, _, validateQueries := setup(t)

	wasCalled := false
	lastModified := clock.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.False(t, wasCalled, "The service did not serve the request from cache")
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Last-Modified", lastModified)
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
		wasCalled = true
	}))
	t.Cleanup(srv.Close)

	// Initial Query
	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
			"Last-Modified":  []string{lastModified},
		},
		resp.Header,
	)

	// Second Query
	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"1"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
			"Last-Modified":  []string{lastModified},
		},
		resp.Header,
	)

	validateQueries([]string{"miss", "hit"})
}

func TestClientRespectsVaryHeadersAndCachesAll(t *testing.T) {
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)

	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count += 1
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
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
			nil,
		)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, fmt.Sprintf("Hello %d!", count), body)

		expectedHeader := http.Header{
			"Cache-Control":  []string{"public, max-age=30"},
			"Content-Length": []string{"8"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Vary":           []string{"Count"},
		}
		if date == nil {
			expectedHeader["Date"] = []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)}
		} else {
			expectedHeader["Date"] = date
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

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"9dea94da2f7eb6112119b81792afb3bc0f18d19d0b6d5cc1ca3a51ebeef7b670",
				200,
				http.Header{
					"Cache-Control":  []string{"public, max-age=30"},
					"Content-Length": []string{"8"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-2 * time.Second).Format(http.TimeFormat),
					},
					"Vary": []string{"Count"},
				},
				http.Header{"Count": []string{"1"}},
				clock.Now().Add(-2 * time.Second).Local(),
			},
			{
				"bab02792998098aa075831b5c79424be14f4d50f316cf555d4d54250258dda6a",
				200,
				http.Header{
					"Cache-Control":  []string{"public, max-age=30"},
					"Content-Length": []string{"8"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
					"Vary": []string{"Count"},
				},
				http.Header{"Count": []string{"2"}},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss", "miss", "hit", "hit"})
}

func TestValidationEtag(t *testing.T) {
	t.Parallel()

	getEtag := func(t string) string {
		switch t {
		case "weak":
			return "W/\"Hello\""
		case "strong":
			return "\"Hello\""
		default:
			panic("BUG")
		}
	}

	for _, originalEtagType := range []string{"weak", "strong"} {
		for _, validationEtagType := range []string{"weak", "strong"} {
			t.Run(originalEtagType+"/"+validationEtagType, func(t *testing.T) {
				t.Parallel()

				originalEtag := getEtag(originalEtagType)
				validationEtag := getEtag(validationEtagType)

				client, clock, validateCache, validateQueries := setup(t)

				srv := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Add("Cache-Control", "public, no-cache")
						w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
						clock.Advance()

						if slices.ContainsFunc(
							r.Header["If-None-Match"],
							func(e string) bool { return e == originalEtag },
						) {
							w.Header().Add("Etag", validationEtag)
							w.Header().Add("Stale", "1")
							w.WriteHeader(http.StatusNotModified)
							return
						}

						w.Header().Add("Etag", originalEtag)
						_, err := w.Write([]byte("Hello!"))
						assert.NoError(t, err)
					}),
				)
				t.Cleanup(srv.Close)

				// First request should get the answer
				resp1, body := makeRequest( //nolint:bodyclose
					t,
					client,
					http.MethodGet,
					srv.URL,
					http.Header{},
					nil,
				)
				assert.Equal(t, 200, resp1.StatusCode)
				assert.Equal(t, "Hello!", body)
				assert.Equal(
					t,
					http.Header{
						"Cache-Control":  []string{"public, no-cache"},
						"Content-Length": []string{"6"},
						"Content-Type":   []string{"text/plain; charset=utf-8"},
						"Date": []string{
							clock.Now().Add(-time.Second).Format(http.TimeFormat),
						},
						"Etag": []string{originalEtag},
					},
					resp1.Header,
				)

				// Second request should revalidate
				resp2, body := makeRequest( //nolint:bodyclose
					t,
					client,
					http.MethodGet,
					srv.URL,
					http.Header{},
					nil,
				)
				assert.Equal(t, 200, resp2.StatusCode)
				assert.Equal(t, "Hello!", body)
				assert.Equal(
					t,
					http.Header{
						"Age":            []string{"1"},
						"Cache-Control":  []string{"public, no-cache"},
						"Content-Length": []string{"6"},
						"Content-Type":   []string{"text/plain; charset=utf-8"},
						"Date": []string{
							clock.Now().Add(-time.Second).Format(http.TimeFormat),
						},
						"Etag":  []string{validationEtag},
						"Stale": []string{"1"},
					},
					resp2.Header,
				)

				validateCache(map[string]CachedResponses{
					"GET+" + srv.URL: {
						{
							"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
							200,
							http.Header{
								"Cache-Control":  []string{"public, no-cache"},
								"Content-Length": []string{"6"},
								"Content-Type":   []string{"text/plain; charset=utf-8"},
								"Date": []string{
									clock.Now().Add(-time.Second).Format(http.TimeFormat),
								},
								"Etag":  []string{validationEtag},
								"Stale": []string{"1"},
							},
							http.Header{},
							clock.Now().Add(-time.Second).Local(),
						},
					},
				})

				validateQueries([]string{"miss", "revalidated"})
			})
		}
	}
}

func TestValidationLastModified(t *testing.T) {
	t.Parallel()
	client, clock, validateCache, validateQueries := setup(t)

	lastModified := time.Now().UTC().Format(http.TimeFormat)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
			clock.Advance()
			w.Header().Add("Last-Modified", lastModified)

			if slices.ContainsFunc(
				r.Header["If-Modified-Since"],
				func(m string) bool { return m == lastModified },
			) {
				w.Header().Add("Stale", "1")
				w.WriteHeader(http.StatusNotModified)
				return
			}

			_, err := w.Write([]byte("Hello!"))
			assert.NoError(t, err)
		}),
	)
	t.Cleanup(srv.Close)

	// First request should get the answer
	resp1, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		http.Header{},
		nil,
	)
	assert.Equal(t, 200, resp1.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
			"Last-Modified":  []string{lastModified},
		},
		resp1.Header,
	)

	// Second request should revalidate
	resp2, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		http.Header{},
		nil,
	)
	assert.Equal(t, 200, resp2.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"1"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
			"Last-Modified":  []string{lastModified},
			"Stale":          []string{"1"},
		},
		resp2.Header,
	)

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
					"Last-Modified": []string{lastModified},
					"Stale":         []string{"1"},
				},
				http.Header{},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})

	validateQueries([]string{"miss", "revalidated"})
}

func TestClientReturnsResponseFromCacheIfDisconnected(t *testing.T) {
	t.Parallel()

	client, clock, _, validateQueries := setup(t)

	wasCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wasCalled {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}

		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Cache-Control", "public, max-age=0")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
		wasCalled = true
	}))
	t.Cleanup(srv.Close)

	// Initial Query
	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
		},
		resp.Header,
	)

	// Second query getting a 5XX, should be served by the cache
	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"1"},
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-time.Second).Format(http.TimeFormat)},
		},
		resp.Header,
	)

	clock.Advance()

	// Third Query, should still be served by the cache
	srv.Close()

	resp, body = makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)
	assert.Equal(
		t,
		http.Header{
			"Age":            []string{"2"},
			"Cache-Control":  []string{"public, max-age=0"},
			"Content-Length": []string{"6"},
			"Content-Type":   []string{"text/plain; charset=utf-8"},
			"Date":           []string{clock.Now().Add(-2 * time.Second).Format(http.TimeFormat)},
		},
		resp.Header,
	)

	validateQueries([]string{"miss", "hit", "hit"})
}

func TestClientTriesUpstreamCachesFirst(t *testing.T) {
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Cache-Control", "public")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	upstreamCache, err := url.Parse(srv.URL)
	require.NoError(t, err)

	resp, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		"https://invalid.test",
		nil,
		[]*url.URL{upstreamCache},
	)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	validateCache(map[string]CachedResponses{
		"GET+https://invalid.test": {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
				},
				http.Header{},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss"})
}

func TestClientIgnoresErrorsFromUpstreamCaches(t *testing.T) {
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Cache-Control", "public")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	upstreamCache, err := url.Parse("https://invalid.test")
	require.NoError(t, err)

	resp, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		nil,
		[]*url.URL{upstreamCache},
	)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
				},
				http.Header{},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss"})
}

func TestClientRetriesQueryWithNoConditionalsIfUnableToFigureOut(t *testing.T) {
	// Some Services don't implement http caching properly, e.g. ghcr.io
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)
	count := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count += 1
		w.Header().Add("Cache-Control", "public")
		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Add("Etag", "123")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		http.Header{},
		nil,
	)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	// And a second time
	resp2, body2 := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		http.Header{},
		nil,
	)
	assert.Equal(t, 200, resp2.StatusCode)
	assert.Equal(t, "Hello!", body2)

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-3 * time.Second).Format(http.TimeFormat),
					},
					"Etag": []string{"123"},
				},
				http.Header{},
				clock.Now().Add(-3 * time.Second).Local(),
			},
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Cache-Control":  []string{"public"},
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-time.Second).Format(http.TimeFormat),
					},
					"Etag": []string{"123"},
				},
				http.Header{},
				clock.Now().Add(-time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss", "miss"})
	assert.Equal(t, 3, count, "Upstream should have been called 3 times")
}

func TestClientPassesThroughConditionalResponseIfNoCacheMatch(t *testing.T) {
	t.Parallel()

	client, clock, validateCache, validateQueries := setup(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var matches []string
		if ifNoneMatch := r.Header["If-None-Match"]; len(ifNoneMatch) != 0 {
			matches = strings.Split(ifNoneMatch[0], ", ")
		}

		w.Header().Add("Date", clock.Now().Format(http.TimeFormat))
		clock.Advance()
		w.Header().Add("Cache-Control", "must-revalidate")
		if slices.Contains(matches, "etag-match") {
			w.Header().Add("Etag", "etag-match")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Add("Etag", "no-match")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	resp, body := makeRequest(t, client, http.MethodGet, srv.URL, nil, nil) //nolint:bodyclose
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Hello!", body)

	resp2, body := makeRequest( //nolint:bodyclose
		t,
		client,
		http.MethodGet,
		srv.URL,
		http.Header{"If-None-Match": []string{"etag-match"}},
		nil,
	)
	assert.Equal(t, http.StatusNotModified, resp2.StatusCode)
	assert.Empty(t, body)

	validateCache(map[string]CachedResponses{
		"GET+" + srv.URL: {
			{
				"52ba594099ad401d60094149fb941a870204d878a522980229e0df63d1c4b7ec",
				200,
				http.Header{
					"Content-Length": []string{"6"},
					"Content-Type":   []string{"text/plain; charset=utf-8"},
					"Date": []string{
						clock.Now().Add(-2 * time.Second).Format(http.TimeFormat),
					},
					"Etag":          []string{"no-match"},
					"Cache-Control": []string{"must-revalidate"},
				},
				http.Header{},
				clock.Now().Add(-2 * time.Second).Local(),
			},
		},
	})
	validateQueries([]string{"miss", "miss"})
}

func TestClientAddsConditionalQueriesInformation(t *testing.T) {
	t.Parallel()

	originalLastModified := time.Now().UTC().Add(-time.Hour).Format(http.TimeFormat)
	cachedModifiedSinceNewer := time.Now().UTC().Add(-2 * time.Hour).Format(http.TimeFormat)
	cachedModifiedSinceOlder := time.Now().UTC().Add(-3 * time.Hour).Format(http.TimeFormat)

	for _, tc := range []struct {
		description                                              string
		incomingHeaders                                          http.Header
		cachedResponses                                          CachedResponses
		expectedHeaders                                          http.Header
		hasConditionalInformation, wasOriginalRequestConditional bool
	}{
		{
			"handle-no-entries",
			http.Header{"Other": {"hello"}},
			CachedResponses{{}},
			http.Header{"Other": {"hello"}},
			false,
			false,
		},
		{
			"handle-request-conditional-only",
			http.Header{"If-None-Match": {"123"}, "If-Modified-Since": {originalLastModified}},
			CachedResponses{{}},
			http.Header{"If-None-Match": {"123"}, "If-Modified-Since": {originalLastModified}},
			false,
			true,
		},
		{
			"add-none-match",
			http.Header{},
			CachedResponses{{Headers: http.Header{"Etag": {"123"}}}},
			http.Header{"If-None-Match": {"123"}},
			true,
			false,
		},
		{
			"merges-none-match",
			http.Header{"If-None-Match": {"request"}},
			CachedResponses{{Headers: http.Header{"Etag": {"123"}}}, {Headers: http.Header{"Etag": {"234"}}}},
			http.Header{"If-None-Match": {"123, 234, request"}},
			true,
			true,
		},
		{
			"add-if-modified-since",
			http.Header{},
			CachedResponses{{Headers: http.Header{"Last-Modified": {cachedModifiedSinceNewer}}}},
			http.Header{"If-Modified-Since": {cachedModifiedSinceNewer}},
			true,
			false,
		},
		{
			"prefers-latest-if-modified-since",
			http.Header{},
			CachedResponses{
				{Headers: http.Header{"Last-Modified": {cachedModifiedSinceNewer}}},
				{Headers: http.Header{"Last-Modified": {cachedModifiedSinceOlder}}},
			},
			http.Header{"If-Modified-Since": {cachedModifiedSinceNewer}},
			true,
			false,
		},
		{
			"no-override-if-modified-since",
			http.Header{"If-Modified-Since": {originalLastModified}},
			CachedResponses{
				{Headers: http.Header{"Last-Modified": {cachedModifiedSinceNewer}}},
			},
			http.Header{"If-Modified-Since": {originalLastModified}},
			false,
			true,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			logger := testutils.TestLogger(t)
			cache, err := NewCache(
				t.TempDir(),
				units.Bytes{Bytes: 100},
				units.Bytes{Bytes: 1000},
				logger,
			)
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, cache.Close()) })

			c := New(
				&http.Client{Transport: &http.Transport{}},
				cache,
				logger,
				true,
				nil,
				time.Now,
				time.Since,
			)

			req := &http.Request{Header: tc.incomingHeaders}
			hasConditional, wasConditional := c.addConditionalRequestInformation(
				req,
				&database.Entry[CachedResponses]{Value: tc.cachedResponses},
			)

			assert.Equal(t, tc.hasConditionalInformation, hasConditional)
			assert.Equal(t, tc.wasOriginalRequestConditional, wasConditional)
			assert.Equal(t, tc.expectedHeaders, req.Header)
		})
	}
}
