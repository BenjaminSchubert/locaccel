package httpcaching

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func TestCanGetVaryHeadersNames(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		description string
		headers     http.Header
		expected    []string
	}{
		{"no-vary", http.Header{}, []string{}},
		{"simple-vary", http.Header{"Vary": []string{"Count"}}, []string{"Count"}},
		{"comma-separated-vary", http.Header{"Vary": []string{"Count, Value"}}, []string{"Count", "Value"}},
		{"multiple-vary", http.Header{"Vary": []string{"Count", "Value"}}, []string{"Count", "Value"}},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			normalized := getVaryHeaderNames(tc.headers)
			require.Equal(t, tc.expected, normalized)
		})
	}
}

func TestExtractVaryHeaders(t *testing.T) {
	t.Parallel()

	headers := ExtractVaryHeaders(
		http.Header{"Foo": []string{"1", "2"}, "Bar": []string{"hello"}},
		http.Header{"Vary": []string{"Foo", "Other"}},
	)
	require.Equal(t, http.Header{"Foo": []string{"1, 2"}, "Other": nil}, headers)
}

func TestMatchVaryHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		description string
		reqHeaders  http.Header
		varyHeaders http.Header
		expected    bool
	}{
		{"no-vary", http.Header{"Foo": []string{}}, http.Header{}, true},
		{"vary-wildcard", http.Header{}, http.Header{"*": nil}, false},
		{"vary-not-matching", http.Header{"Foo": []string{"one"}}, http.Header{"Foo": []string{"two"}}, false},
		{"vary-missing", http.Header{}, http.Header{"Foo": []string{"one"}}, false},
		{"vary-matching", http.Header{"Foo": []string{"one"}, "Bar": []string{"two"}}, http.Header{"Foo": []string{"one"}, "Baz": nil}, true},
		{"vary-matching-normalized", http.Header{"Foo": []string{"one, two", "three"}}, http.Header{"Foo": []string{"one, two, three"}}, true},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			match := MatchVaryHeaders(tc.reqHeaders, tc.varyHeaders, testutils.TestLogger(t))
			require.Equal(t, tc.expected, match)
		})
	}
}
