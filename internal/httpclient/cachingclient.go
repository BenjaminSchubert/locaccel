package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/filecache"
	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpcaching"
)

var errNoMatchVaryHeaders = errors.New("vary headers don't match")

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
	logger zerolog.Logger
}

func New(client *http.Client, cachePath string, logger zerolog.Logger) (*Client, error) {
	cache, err := filecache.NewFileCache(
		path.Join(cachePath, "cache"),
		logger.With().Str("component", "filecache").Logger(),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize file cache: %w", err)
	}

	db, err := database.NewDatabase[[]cachedResponse](
		path.Join(cachePath, "db"),
		logger.With().Str("component", "database").Logger(),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize database: %w", err)
	}

	return &Client{client, db, cache, logger}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

func buildKey(req *http.Request) string {
	return req.Method + "+" + req.URL.String()
}

func (c *Client) serveFromCache(
	req *http.Request,
	cacheKey string,
) (*http.Response, *database.Entry[[]cachedResponse], error) {
	dbEntry, err := c.db.Get(cacheKey)
	if err != nil {
		return nil, nil, err
	}

	for _, resp := range dbEntry.Value {
		if httpcaching.MatchVaryHeaders(req.Header, resp.VaryHeaders, c.logger) {
			// FIXME: we should not take any response, we should see if one is more prefered
			//		  See https://datatracker.ietf.org/doc/html/rfc9111#section-4.1

			age, isFresh := httpcaching.IsFresh(
				resp.Headers,
				resp.TimeAtRequestCreated,
				resp.TimeAtResponseReceived,
				c.logger,
			)
			if !isFresh {
				// FIXME: implement re-validation
				c.logger.Warn().
					Stringer("url", req.URL).
					Msg("can't use cached response as it is stale")
				continue
			}
			body, err := c.cache.Open(resp.ContentHash)
			if err != nil {
				// FIXME: is it possible that there would be an other matching the headers?
				return nil, dbEntry, err
			}

			c.logger.Debug().Stringer("url", req.URL).Msg("serving response from cache")
			resp.Headers.Add("Age", strconv.FormatFloat(age.Seconds(), 'f', 0, 64))
			return &http.Response{
				Body:       body,
				Header:     resp.Headers,
				StatusCode: resp.StatusCode,
			}, dbEntry, nil
		}
	}

	// FIXME: Validate the query(ies) instead of just giving up
	return nil, dbEntry, errNoMatchVaryHeaders
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// We only support caching GET requests
	if req.Method != http.MethodGet {
		return c.client.Do(req)
	}

	cacheKey := buildKey(req)

	cachedResp, dbEntry, err := c.serveFromCache(req, cacheKey)
	if err == nil {
		return cachedResp, nil
	}
	c.logger.Debug().Stringer("url", req.URL).Err(err).Msg("unable to serve from cache")

	timeAtRequestCreated := time.Now().UTC()
	resp, err := c.client.Do(req)
	if err != nil {
		return resp, err
	}
	timeAtResponseReceived := time.Now().UTC()

	if !httpcaching.IsCacheable(resp) {
		return resp, nil
	}

	// Ensure the Date header is valid,
	// as per https://datatracker.ietf.org/doc/html/rfc9110#name-date
	if _, err := http.ParseTime(resp.Header.Get("Date")); err != nil {
		c.logger.Debug().Err(err).Msg("invalid Date header replaced")
		resp.Header["Date"] = []string{time.Now().UTC().Format(http.TimeFormat)}
	}

	ingestor := c.cache.SetupIngestion(resp.Body, func(hash string) {
		cacheResp := cachedResponse{
			hash,
			resp.StatusCode,
			httpcaching.FilterUncacheableHeaders(resp),
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
			c.logger.Error().
				Stringer("url", req.URL).
				Err(err).
				Msg("Error saving entry in the database")
		}
	})
	resp.Body = ingestor

	return resp, nil
}
