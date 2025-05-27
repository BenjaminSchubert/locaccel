package httpclient

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpcaching"
	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpheaders"
)

var errNoMatchingEntryInCache = errors.New("no entries match Etag or Last-Modified")

type Client struct {
	client    *http.Client
	db        *database.Database[CachedResponses, *CachedResponses]
	cache     *filecache.FileCache
	isPrivate bool
	notify    func(r *http.Request, status string)
}

func New(
	client *http.Client,
	cache *Cache,
	logger *zerolog.Logger,
	isPrivate bool,
	notify func(r *http.Request, status string),
) *Client {
	return &Client{client, cache.db, cache.cache, isPrivate, notify}
}

func buildKey(req *http.Request) string {
	return req.Method + "+" + req.URL.String()
}

func (c *Client) selectResponseCandidates(
	req *http.Request,
	dbEntry *database.Entry[CachedResponses],
	logger *zerolog.Logger,
) CachedResponses {
	candidates := CachedResponses{}

	for _, resp := range dbEntry.Value {
		if httpcaching.MatchVaryHeaders(req.Header, resp.VaryHeaders, logger) {
			candidates = append(candidates, resp)
		}
	}

	return candidates
}

func (c *Client) selectMostRecentCandidates(
	candidates CachedResponses,
	logger *zerolog.Logger,
) CachedResponses {
	mostRecentCandidates := make(CachedResponses, 0, 1)
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
	candidates CachedResponses,
	forceStale bool,
	logger *zerolog.Logger,
) *http.Response {
	// FIXME: most recent is not necessarily most prefered,
	//	      we might want to implement proper preferences
	//		  See https://datatracker.ietf.org/doc/html/rfc9111#section-4.1
	mostRecentCandidates := c.selectMostRecentCandidates(candidates, logger)

	for _, resp := range mostRecentCandidates {
		cacheControl, err := httpcaching.ParseCacheControlDirective(
			resp.Headers["Cache-Control"],
			logger,
		)
		if err != nil {
			logger.Warn().Err(err).Msg("unable to parse cache control directives")
		}

		if cacheControl.NoCache || cacheControl.MustRevalidate {
			continue
		}

		age, isFresh := httpcaching.IsFresh(
			resp.Headers,
			cacheControl,
			resp.TimeAtResponseCreation,
			logger,
		)
		if isFresh || forceStale {
			body, err := c.cache.Open(resp.ContentHash, logger)
			if err != nil {
				logger.Warn().Err(err).Msg("Entry has been pruned from the cache already")
				continue
			}

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
	dbEntry *database.Entry[CachedResponses],
	forceStale bool,
	logger *zerolog.Logger,
) *http.Response {
	candidates := c.selectResponseCandidates(req, dbEntry, logger)
	if len(candidates) == 0 {
		// No candidate can be used
		return nil
	}

	resp := c.serveFromCachedCandidates(candidates, forceStale, logger)
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
		c.notify(req, "miss")
		return resp, err
	}

	cacheKey := buildKey(req)

	dbEntry, err := c.db.Get(cacheKey)
	if err == nil {
		resp := c.serveFromCache(req, dbEntry, false, logger)
		if resp != nil {
			logger.Debug().Msg("serving response from cache")
			c.notify(req, "hit")
			return resp, nil
		}
	} else if !errors.Is(err, database.ErrKeyNotFound) {
		logger.Debug().Err(err).Msg("unable to retrieve entry from database, no response fresh")
	}

	hasConditionalInformation := false
	if dbEntry != nil {
		hasConditionalInformation = c.addConditionalRequestInformation(req, dbEntry)
	}

	logger.Debug().Msg("unable to serve from cache")

	resp, timeAtRequestCreated, timeAtResponseReceived, err := c.forwardRequest(req, logger)
	if err != nil || resp.StatusCode >= 500 {
		if dbEntry != nil {
			if cRep := c.serveFromCache(req, dbEntry, true, logger); cRep != nil {
				logger.Warn().
					Err(err).
					Msg("unable to contact upstream, serving stale response from cache")
				c.notify(req, "hit")
				return cRep, nil
			}
		}
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
			c.notify(req, "revalidated")
			return resp, nil
		}
	}

	c.notify(req, "miss")

	if !httpcaching.IsCacheable(resp, c.isPrivate, logger) {
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
	dbEntry *database.Entry[CachedResponses],
) bool {
	etags := []string{}
	lastModified := []string{}

	for _, entry := range dbEntry.Value {
		etags = append(etags, entry.Headers["Etag"]...)
		lastModified = append(lastModified, entry.Headers["Last-Modified"]...)

	}

	if len(etags) != 0 {
		originalEtag := req.Header["If-None-Match"]
		if originalEtag != nil {
			etags = append(etags, originalEtag...)
		}

		// Some servers don't support more than one If-None-Match headers
		req.Header["If-None-Match"] = []string{strings.Join(etags, ", ")}
	}

	if len(lastModified) != 0 {
		originalLastModified := req.Header["If-Unmodified-Since"]
		if originalLastModified != nil {
			lastModified = append(lastModified, originalLastModified...)
		}

		req.Header["If-Unmodified-Since"] = []string{strings.Join(lastModified, ", ")}
	}

	return len(etags) != 0 || len(lastModified) != 0
}

func (c *Client) forwardRequest(
	req *http.Request,
	logger *zerolog.Logger,
) (resp *http.Response, timeAtRequestCreated, timeAtResponseReceived time.Time, err error) {
	removeHopByHopHeaders(req.Header)

	timeAtRequestCreated = time.Now().UTC()
	resp, err = c.client.Do(req)
	timeAtResponseReceived = time.Now().UTC()

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
	dbEntry *database.Entry[CachedResponses],
	logger *zerolog.Logger,
) io.ReadCloser {
	return c.cache.SetupIngestion(
		resp.Body,
		func(hash string) {
			var err error

			cacheResp := CachedResponse{
				hash,
				resp.StatusCode,
				resp.Header,
				httpcaching.ExtractVaryHeaders(req.Header, resp.Header),
				httpcaching.GetEstimatedResponseCreation(
					resp.Header,
					timeAtRequestCreated,
					timeAtResponseReceived,
					logger,
				),
			}

			if dbEntry != nil {
				dbEntry.Value = append(dbEntry.Value, cacheResp)
				err = c.db.Save(cacheKey, dbEntry)
			} else {
				err = c.db.New(cacheKey, CachedResponses{cacheResp})
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
	dbEntry *database.Entry[CachedResponses],
	resp *http.Response,
	timeAtRequestCreated, timeAtResponseReceived time.Time,
	logger *zerolog.Logger,
) (*http.Response, error) {
	if etag := resp.Header.Get("Etag"); etag != "" {
		for idx, cachedResp := range dbEntry.Value {
			if httpheaders.EtagsMatch(etag, cachedResp.Headers.Get("Etag")) {
				return c.refreshResponseAndServe(
					cacheKey,
					dbEntry,
					idx,
					resp,
					timeAtRequestCreated,
					timeAtResponseReceived,
					logger,
				), nil
			}
		}
	}

	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		for idx, cachedResp := range dbEntry.Value {
			if lastModified == cachedResp.Headers.Get("Last-Modified") {
				return c.refreshResponseAndServe(
					cacheKey,
					dbEntry,
					idx,
					resp,
					timeAtRequestCreated,
					timeAtResponseReceived,
					logger,
				), nil
			}
		}
	}

	return resp, errNoMatchingEntryInCache
}

func (c *Client) refreshResponseAndServe(
	cacheKey string,
	dbEntry *database.Entry[CachedResponses],
	idx int,
	resp *http.Response,
	timeAtRequestCreated, timeAtResponseReceived time.Time,
	logger *zerolog.Logger,
) *http.Response {
	cachedResp := dbEntry.Value[idx]

	for key, val := range resp.Header {
		if key != "Content-Length" {
			cachedResp.Headers[key] = val
		}
	}
	cachedResp.TimeAtResponseCreation = httpcaching.GetEstimatedResponseCreation(
		resp.Header,
		timeAtRequestCreated,
		timeAtResponseReceived,
		logger,
	)

	resp.Header = cachedResp.Headers

	if err := c.db.Save(cacheKey, dbEntry); err != nil {
		logger.Error().Err(err).Msg("Error updating the entry in the cache")
	}

	body, err := c.cache.Open(cachedResp.ContentHash, logger)
	if err != nil {
		logger.Panic().Err(err).Msg("Unable to serve cached entry")
	}
	if err := resp.Body.Close(); err != nil {
		logger.Error().Err(err).Msg("Error closing upstream request body")
	}

	r := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     cachedResp.Headers.Clone(),
	}

	age := httpcaching.GetCurrentAge(cachedResp.TimeAtResponseCreation)
	r.Header.Set("Age", strconv.FormatFloat(age.Seconds(), 'f', 0, 64))

	return r
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
