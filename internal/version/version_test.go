package version_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/version"
)

func TestCanGetVersion(t *testing.T) {
	t.Parallel()

	require.Equal(t, "(devel)", version.Get())
}
