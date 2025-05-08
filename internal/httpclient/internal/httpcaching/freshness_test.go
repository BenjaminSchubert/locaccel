package httpcaching

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func TestGetFreshness(t *testing.T) {
	for _, tc := range []struct {
		description string
		headers     http.Header
		expected    int
	}{
		{
			"s-max-age-precedence",
			http.Header{
				"Cache-Control": []string{"s-max-age=1, max-age=2"},
				"Expires":       []string{"Sun, 01 Jan 2012 12:00:00 GMT"},
			},
			1,
		},
		{
			"max-age-over-expires",
			http.Header{
				"Cache-Control": []string{"max-age=2"},
				"Expires":       []string{"Sun, 01 Jan 2012 12:00:00 GMT"},
			},
			2,
		},
		{
			"expires",
			http.Header{
				"Date":    []string{"Sun, 01 Jan 2012 11:00:00 GMT"},
				"Expires": []string{"Sun, 01 Jan 2012 12:00:00 GMT"},
			},
			3600,
		},
		{
			"ignores-misformed-control",
			http.Header{
				"Cache-Control": []string{"s-max-age=hello"},
				"Date":          []string{"Sun, 01 Jan 2012 11:00:00 GMT"},
				"Expires":       []string{"Sun, 01 Jan 2012 12:00:00 GMT"},
			},
			3600,
		},
		{
			"default-if-invalid-expires",
			http.Header{
				"Expires": []string{"hi"},
			},
			0,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			freshness := getFreshnessLifetime(tc.headers, testutils.TestLogger(t))
			require.Equal(t, time.Second*time.Duration(tc.expected), freshness)
		})
	}
}

func TestGetCurrentAge(t *testing.T) {
	age := getCurrentAge(
		http.Header{
			"Age":  []string{"60"},
			"Date": []string{time.Now().UTC().Add(-time.Second * 120).Format(http.TimeFormat)},
		},
		time.Now().Add(-time.Second*40),
		time.Now().Add(-time.Second*30),
		testutils.TestLogger(t),
	)
	require.Equal(t, time.Second*time.Duration(120), age)
}

func TestIsFresh(t *testing.T) {
	age, isFresh := IsFresh(
		http.Header{
			"Age":           []string{"60"},
			"Cache-Control": []string{"s-max-age=300"},
			"Date": []string{
				time.Now().UTC().Add(-time.Second * 120).Format(http.TimeFormat),
			},
		},
		time.Now().Add(-time.Second*40),
		time.Now().Add(-time.Second*30),
		testutils.TestLogger(t),
	)
	assert.Equal(t, time.Second*120, age)
	assert.Equal(t, true, isFresh)
}

func TestIsNotFresh(t *testing.T) {
	age, isFresh := IsFresh(
		http.Header{
			"Age":           []string{"60"},
			"Cache-Control": []string{"s-max-age=30"},
			"Date": []string{
				time.Now().UTC().Add(-time.Second * 120).Format(http.TimeFormat),
			},
		},
		time.Now().Add(-time.Second*40),
		time.Now().Add(-time.Second*30),
		testutils.TestLogger(t),
	)
	assert.Equal(t, time.Second*120, age)
	assert.Equal(t, false, isFresh)
}
