package units_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/units"
)

func TestCanParseQuotaBytes(t *testing.T) {
	t.Parallel()

	quota := units.DiskQuota{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("10"))

	err := decoder.Decode(&quota)
	require.NoError(t, err)
	require.Equal(t, units.NewDiskQuotaInBytes(units.Bytes{Bytes: 10}), quota)
}

func TestCanParseQuotaPercent(t *testing.T) {
	t.Parallel()

	quota := units.DiskQuota{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("10 %"))

	err := decoder.Decode(&quota)
	require.NoError(t, err)
	require.Equal(t, units.NewDiskQuotaInPercent(10), quota)
}

func TestCanGetBytesFromPercent(t *testing.T) {
	t.Parallel()

	quota := units.NewDiskQuotaInPercent(10)
	b, err := quota.Bytes(t.TempDir())
	require.NoError(t, err)
	require.GreaterOrEqual(t, b.Bytes, int64(10))
}
