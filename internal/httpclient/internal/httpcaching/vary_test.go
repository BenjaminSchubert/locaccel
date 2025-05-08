package httpcaching

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/logging"
)

func TestNormalizeVaryHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		description string
		headers     http.Header
		expected    map[string]struct{}
	}{
		{"no-vary", http.Header{}, map[string]struct{}{}},
		{"simple-vary", http.Header{"Vary": []string{"Count"}}, map[string]struct{}{"Count": {}}},
		{"comma-separated-vary", http.Header{"Vary": []string{"Count, Value"}}, map[string]struct{}{"Count": {}, "Value": {}}},
		{"multiple-vary", http.Header{"Vary": []string{"Count", "Value"}}, map[string]struct{}{"Count": {}, "Value": {}}},
	} {
		t.Run(tc.description, func(t *testing.T) {
			normalized := normalizeVaryHeaders(tc.headers)
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
	require.Equal(t, http.Header{"Foo": []string{"1", "2"}, "Other": nil}, headers)
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
		{"vary-not-same-length", http.Header{"Foo": []string{"one"}}, http.Header{"Foo": []string{"one", "one"}}, false},
		{"vary-not-matching", http.Header{"Foo": []string{"one"}}, http.Header{"Foo": []string{"two"}}, false},
		{"vary-matching", http.Header{"Foo": []string{"one"}, "Bar": []string{"two"}}, http.Header{"Foo": []string{"one"}, "Baz": nil}, true},
	} {
		t.Run(tc.description, func(t *testing.T) {
			match := MatchVaryHeaders(tc.reqHeaders, tc.varyHeaders, logging.TestLogger(t))
			require.Equal(t, tc.expected, match)
		})
	}
}
