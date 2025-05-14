package httpcaching_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpcaching"
)

func TestCanParseValidHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		header   string
		expected httpcaching.CacheControlResponseDirective
	}{
		{"immutable", httpcaching.CacheControlResponseDirective{Immutable: true}},
		{"max-age=123", httpcaching.CacheControlResponseDirective{MaxAge: 123 * time.Second}},
		{"must-revalidate", httpcaching.CacheControlResponseDirective{MustRevalidate: true}},
		{"must-understand", httpcaching.CacheControlResponseDirective{MustUnderstand: true}},
		{"no-cache", httpcaching.CacheControlResponseDirective{NoCache: true}},
		{"no-cache=123", httpcaching.CacheControlResponseDirective{NoCache: true}},
		{"no-store", httpcaching.CacheControlResponseDirective{NoStore: true}},
		{"no-transform", httpcaching.CacheControlResponseDirective{NoTransform: true}},
		{"private", httpcaching.CacheControlResponseDirective{Private: true}},
		{"private=123", httpcaching.CacheControlResponseDirective{Private: true}},
		{"proxy-revalidate", httpcaching.CacheControlResponseDirective{ProxyRevalidate: true}},
		{"public", httpcaching.CacheControlResponseDirective{Public: true}},
		{"s-max-age=12", httpcaching.CacheControlResponseDirective{SMaxAge: 12 * time.Second}},
		{"stale-while-revalidate=10", httpcaching.CacheControlResponseDirective{StaleWhileRevalidate: 10 * time.Second}},
		{"stale-if-error=10", httpcaching.CacheControlResponseDirective{StaleIfError: 10 * time.Second}},
	} {
		t.Run(tc.header, func(t *testing.T) {
			t.Parallel()

			result, err := httpcaching.ParseCacheControlDirective([]string{tc.header})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestErrorsOnInvalidHeaders(t *testing.T) {
	t.Parallel()

	for _, header := range []string{
		"max-age=hello",
		"s-max-age=hello",
		"stale-while-revalidate=hello",
		"stale-if-error=hello",
	} {
		t.Run(header, func(t *testing.T) {
			t.Parallel()

			_, err := httpcaching.ParseCacheControlDirective([]string{header})
			require.ErrorIs(t, err, httpcaching.ErrInvalidArgument)
		})
	}
}

func TestIgnoreInvalidDirective(t *testing.T) {
	t.Parallel()

	result, err := httpcaching.ParseCacheControlDirective([]string{"unknown"})
	require.NoError(t, err)
	assert.Equal(t, httpcaching.CacheControlResponseDirective{}, result)
}

func TestCanComposeMultipleHeaders(t *testing.T) {
	t.Parallel()

	result, err := httpcaching.ParseCacheControlDirective(
		[]string{"max-age=123", "must-revalidate, no-cache"},
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		httpcaching.CacheControlResponseDirective{
			MaxAge:         123 * time.Second,
			MustRevalidate: true,
			NoCache:        true,
		},
		result,
	)
}
