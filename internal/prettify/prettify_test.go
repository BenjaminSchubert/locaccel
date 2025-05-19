package prettify_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/prettify"
)

func TestBytes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value    int64
		expected string
	}{
		{1000, "1000B"},
		{1024, "1.00KiB"},
		{8192, "8.00KiB"},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, prettify.Bytes(tc.value))
		})
	}
}
