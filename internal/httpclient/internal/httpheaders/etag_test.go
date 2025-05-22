package httpheaders_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpheaders"
)

func TestEtagHeadersMatch(t *testing.T) {
	t.Parallel()

	for _, tc := range [][]string{{"", ""}, {"W/", ""}, {"", "W/"}, {"W/", "W/"}} {
		t.Run(strings.Join(tc, ","), func(t *testing.T) {
			t.Parallel()

			require.True(t, httpheaders.EtagsMatch(tc[0]+"\"val\"", tc[1]+"\"val\""))
		})
	}
}

func TestEtagHeadersDontMatch(t *testing.T) {
	t.Parallel()
	require.False(t, httpheaders.EtagsMatch("\"one\"", "\"two\""))
}
