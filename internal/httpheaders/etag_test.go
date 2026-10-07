package httpheaders

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitEtagList(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input    string
		expected []string
	}{
		{
			"\"bfc13a64729c4290ef5b2c2730249c88ca92d82d\"",
			[]string{"\"bfc13a64729c4290ef5b2c2730249c88ca92d82d\""},
		},
		{
			"W/\"67ab43\", \"54ed21\", \"7892dd\"",
			[]string{"W/\"67ab43\"", "\"54ed21\"", "\"7892dd\""},
		},
		{
			"*",
			[]string{"*"},
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, splitEtagList(tc.input))
		})
	}
}

func TestEtagHeadersMatch(t *testing.T) {
	t.Parallel()

	for _, tc := range [][]string{{"", ""}, {"W/", ""}, {"", "W/"}, {"W/", "W/"}} {
		t.Run(strings.Join(tc, ","), func(t *testing.T) {
			t.Parallel()

			require.True(t, etagsMatch(tc[0]+"\"val\"", tc[1]+"\"val\""))
		})
	}
}

func TestEtagHeadersDontMatch(t *testing.T) {
	t.Parallel()
	assert.False(t, etagsMatch("\"one\"", "\"two\""))
	assert.False(t, etagsMatch("\"", "\"two\""))
	assert.False(t, etagsMatch("\"one\"", "\""))
}

func TestMatchesEtags(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		description string
		etag        string
		hdr         string
		expected    bool
	}{
		{"exact", "\"abc\"", "\"abc\"", true},
		{"match-in-list", "\"abc\"", "\"abc\",\"def\"", true},
		{"no-match", "\"abc\"", "\"def\",\"ghi\"", false},
		{"weak", "\"abc\"", "W/\"abc\"", true},
		{"weak-reversed", "W/\"abc\"", "\"abc\"", true},
		{"both-weak", "W/\"abc\"", "W/\"abc\"", true},
		{"wildcard", "\"abc\"", "*", true},
		{"no-response", "", "\"abc\"", false},
		{"no-response-wildcard", "", "*", true},
		{"comma-no-split", "\"a,b\"", "\"a\"", false},
		{"comma", "\"a,b\"", "\"a,b\"", true},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, EtagMatchesAny(tc.etag, []string{tc.hdr}))
		})
	}
}
