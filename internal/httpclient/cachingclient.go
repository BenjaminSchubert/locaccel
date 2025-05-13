package httpclient

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpcaching"
)

var errNoMatchingEntryInCache = errors.New("no entries match Etag or Last-Modified")

type cachedResponse struct {
	ContentHash            string
	StatusCode             int
	Headers                http.Header
	VaryHeaders            http.Header
	TimeAtRequestCreated   time.Time
	TimeAtResponseReceived time.Time
}

type Client struct {
	client *http.Client
	db     *database.Database[[]cachedResponse]
	cache  *filecache.FileCache
}

func New(client *http.Client, cachePath string, logger *zerolog.Logger) (*Client, error) {
	cache, err := filecache.NewFileCache(path.Join(cachePath, "cache"))
	if err != nil {
		return nil, fmt.Errorf("unable to initialize file cache: %w", err)
	}

	// Ensure the db logger is not too chatty
	dbLogger := logger.With().Str("component", "database").Logger()
	if dbLogger.GetLevel() < zerolog.InfoLevel {
		dbLogger = dbLogger.Level(zerolog.InfoLevel)
	}

	db, err := database.NewDatabase[[]cachedResponse](
		path.Join(cachePath, "db"),
		&dbLogger,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize database: %w", err)
	}

	return &Client{client, db, cache}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

func buildKey(req *http.Request) string {
	return req.Method + "+" + req.URL.String()
}

func (c *Client) selectResponseCandidates(
	req *http.Request,
	dbEntry *database.Entry[[]cachedResponse],
	logger *zerolog.Logger,
) []cachedResponse {
	candidates := []cachedResponse{}

	for _, resp := range dbEntry.Value {
		if httpcaching.MatchVaryHeaders(req.Header, resp.VaryHeaders, logger) {
			candidates = append(candidates, resp)
		}
	}

	return candidates
}

func (c *Client) selectMostRecentCandidates(
	candidates []cachedResponse,
	logger *zerolog.Logger,
) []cachedResponse {
	mostRecentCandidates := make([]cachedResponse, 0, 1)
	maxDate := time.Time{}

	for _, candidate := range candidates {
		date, err := http.ParseTime(candidate.Headers.Get("Date"))
		if err != nil {
			logger.Error().
				Err(err).
				Msg("BUG: Date header is in an invalid format, which should not happen")
			date = time.Time{}
		}
		if date.After(maxDate) {
			mostRecentCandidates = mostRecentCandidates[:0]
			mostRecentCandidates = append(mostRecentCandidates, candidate)
			maxDate = date
		} else if date.Equal(maxDate) {
			mostRecentCandidates = append(mostRecentCandidates, candidate)
		}
	}

	return mostRecentCandidates
}

func (c *Client) serveFromCachedCandidates(
	candidates []cachedResponse,
	logger *zerolog.Logger,
) *http.Response {
	// FIXME: most recent is not necessarily most prefered,
	//	      we might want to implement proper preferences
	//		  See https://datatracker.ietf.org/doc/html/rfc9111#section-4.1
	mostRecentCandidates := c.selectMostRecentCandidates(candidates, logger)

	for _, resp := range mostRecentCandidates {
		cacheControl, err := httpcaching.ParseCacheControlDirective(resp.Headers["Cache-Control"])
		if err != nil {
			logger.Warn().Err(err).Msg("unable to parse cache control directives")
		}

		if cacheControl.NoCache || cacheControl.MustRevalidate {
			continue
		}

		age, isFresh := httpcaching.IsFresh(
			resp.Headers,
			cacheControl,
			resp.TimeAtRequestCreated,
			resp.TimeAtResponseReceived,
			logger,
		)
		if isFresh {
			body, err := c.cache.Open(resp.ContentHash, logger)
			if err != nil {
				// FIXME: delete entry, it's useless now
				continue
			}

			logger.Debug().Msg("serving response from cache")
			resp.Headers.Set("Age", strconv.FormatFloat(age.Seconds(), 'f', 0, 64))
			return &http.Response{
				Body:       body,
				Header:     resp.Headers,
				StatusCode: resp.StatusCode,
			}
		}
	}

	return nil
}

func (c *Client) serveFromCache(
	req *http.Request,
	dbEntry *database.Entry[[]cachedResponse],
	logger *zerolog.Logger,
) *http.Response {
	candidates := c.selectResponseCandidates(req, dbEntry, logger)
	if len(candidates) == 0 {
		// No candidate can be used
		return nil
	}

	resp := c.serveFromCachedCandidates(candidates, logger)
	if resp != nil {
		return resp
	}

	// All responses are stale, we need to revalidate
	return nil
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	logger := hlog.FromRequest(req)

	// We only support caching GET requests
	if req.Method != http.MethodGet {
		resp, _, _, err := c.forwardRequest(req, logger)
		return resp, err
	}

	cacheKey := buildKey(req)

	dbEntry, err := c.db.Get(cacheKey)
	if err == nil {
		resp := c.serveFromCache(req, dbEntry, logger)
		if resp != nil {
			return resp, nil
		}
	} else if err != database.ErrKeyNotFound {
		logger.Debug().Err(err).Msg("unable to retrieve entry from database, no response fresh")
	}

	hasConditionalInformation := false
	if dbEntry != nil {
		hasConditionalInformation = c.addConditionalRequestInformation(req, dbEntry)
	}

	logger.Debug().Msg("unable to serve from cache")

	// FIXME: use stale entries from cache on 5XX+
	resp, timeAtRequestCreated, timeAtResponseReceived, err := c.forwardRequest(req, logger)
	if err != nil {
		return resp, err
	}

	if hasConditionalInformation && resp.StatusCode == http.StatusNotModified {
		resp, err := c.updateCache(
			cacheKey,
			dbEntry,
			resp,
			timeAtRequestCreated,
			timeAtResponseReceived,
			logger,
		)
		if err != nil && !errors.Is(err, errNoMatchingEntryInCache) {
			// FIXME: check if original request was conditional and return if so
			panic(err)
		}
		if resp != nil {
			logger.Debug().Msg("request re-validated, serving from cache")
			return resp, nil
		}
	}

	if !httpcaching.IsCacheable(resp) {
		logger.Debug().Msg("request is not cacheable")
		return resp, nil
	}

	resp.Body = c.setupIngestion(
		req,
		resp,
		timeAtRequestCreated,
		timeAtResponseReceived,
		cacheKey,
		dbEntry,
		logger,
	)
	return resp, nil
}

func (c *Client) addConditionalRequestInformation(
	req *http.Request,
	dbEntry *database.Entry[[]cachedResponse],
) bool {
	// FIXME: add If-Not-Modified-Since

	etags := []string{}

	for _, entry := range dbEntry.Value {
		etags = append(etags, entry.Headers["Etag"]...)
	}

	if len(etags) != 0 {
		originalEtag := req.Header["If-None-Match"]
		if originalEtag != nil {
			etags = append(etags, originalEtag...)
		}

		// Some servers don't support more than one If-None-Match headers
		req.Header["If-None-Match"] = []string{strings.Join(etags, ", ")}
	}

	return len(etags) != 0
}

func (c *Client) forwardRequest(
	req *http.Request,
	logger *zerolog.Logger,
) (*http.Response, time.Time, time.Time, error) {
	removeHopByHopHeaders(req.Header)

	timeAtRequestCreated := time.Now().UTC()
	resp, err := c.client.Do(req)
	timeAtResponseReceived := time.Now().UTC()

	if err != nil {
		return resp, timeAtRequestCreated, timeAtResponseReceived, err
	}

	removeHopByHopHeaders(resp.Header)

	// Ensure the Date header is valid,
	// as per https://datatracker.ietf.org/doc/html/rfc9110#name-date
	if _, err := http.ParseTime(resp.Header.Get("Date")); err != nil {
		logger.Debug().Err(err).Msg("invalid Date header replaced")
		resp.Header["Date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	}

	return resp, timeAtRequestCreated, timeAtResponseReceived, err
}

func (c *Client) setupIngestion(
	req *http.Request,
	resp *http.Response,
	timeAtRequestCreated, timeAtResponseReceived time.Time,
	cacheKey string,
	dbEntry *database.Entry[[]cachedResponse],
	logger *zerolog.Logger,
) io.ReadCloser {
	return c.cache.SetupIngestion(
		resp.Body,
		func(hash string) {
			var err error

			cacheResp := cachedResponse{
				hash,
				resp.StatusCode,
				resp.Header,
				httpcaching.ExtractVaryHeaders(req.Header, resp.Header),
				timeAtRequestCreated,
				timeAtResponseReceived,
			}

			if dbEntry != nil {
				dbEntry.Value = append(dbEntry.Value, cacheResp)
				err = c.db.Save(cacheKey, dbEntry)
			} else {
				err = c.db.New(cacheKey, []cachedResponse{cacheResp})
			}

			if err != nil {
				logger.Error().Err(err).Msg("Error saving entry in the database")
			} else {
				logger.Debug().Msg("request saved in the database")
			}
		},
		logger,
	)
}

func (c *Client) updateCache(
	cacheKey string,
	dbEntry *database.Entry[[]cachedResponse],
	resp *http.Response,
	timeAtRequestCreated, timeAtResponseReceived time.Time,
	logger *zerolog.Logger,
) (*http.Response, error) {
	if etag := resp.Header.Get("Etag"); etag != "" {
		for _, cachedResp := range dbEntry.Value {
			if cachedResp.Headers.Get("Etag") == etag {
				for key, val := range resp.Header {
					if key != "Content-Length" {
						cachedResp.Headers[key] = val
					}
				}

				if err := c.db.Save(cacheKey, dbEntry); err != nil {
					logger.Error().Err(err).Msg("Error updating the entry in the cache")
				}

				body, err := c.cache.Open(cachedResp.ContentHash, logger)
				if err != nil {
					// FIXME: delete entry, it's useless now
					panic("Unable to serve cached entry")
				}
				if err := resp.Body.Close(); err != nil {
					logger.Error().Err(err).Msg("Error closing upstream request body")
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       body,
					Header:     cachedResp.Headers.Clone(),
				}

				age := httpcaching.GetCurrentAge(
					resp.Header,
					timeAtRequestCreated,
					timeAtResponseReceived,
					logger,
				)
				resp.Header.Set("Age", strconv.FormatFloat(age.Seconds(), 'f', 0, 64))

				return resp, nil
			}
		}
	}

	return resp, errNoMatchingEntryInCache
}

func removeHopByHopHeaders(headers http.Header) {
	// Implements RFC 9111 section 3.1

	// The Connection header field and fields whose names are listed in it are
	// required by Section 7.6.1 of RFC 9110 to be removed before forwarding the
	// message. This MAY be implemented by doing so before storage.
	headers.Del("Connection")

	// Likewise, some fields' semantics require them to be removed before
	// forwarding the message, and this MAY be implemented by doing so before
	// storage; see Section 7.6.1 of RFC 9110 for some examples.
	headers.Del("Proxy-Connection")
	headers.Del("Keep-Alive")
	headers.Del("Te")
	headers.Del("Trailer")
	headers.Del("Transfer-Encoding")
	headers.Del("Upgrade")

	// Header fields that are specific to the proxy that a cache uses when
	// forwarding a request MUST NOT be stored, unless the cache incorporates
	// the identity of the proxy into the cache key. Effectively, this is
	// limited to Proxy-Authenticate (Section 11.7.1 of RFC 9110),
	// Proxy-Authentication-Info (Section 11.7.3 of RFC 9110), and
	// Proxy-Authorization (Section 11.7.2 of RFC 9110)
	headers.Del("Proxy-Authenticate")
	headers.Del("Proxy-Authentication-Info")
	headers.Del("Proxy-Authorization")
}
