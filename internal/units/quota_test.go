package units_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"strings"
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

func TestRejectInvalidQuota(t *testing.T) {
	t.Parallel()

	quota := units.DiskQuota{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("hello"))

	err := decoder.Decode(&quota)
	require.ErrorIs(t, err, units.ErrInvalidQuotaFormat)
}

func TestRejectQuotaBiggerThanMaxFloat(t *testing.T) {
	t.Parallel()

	quota := units.DiskQuota{}
	decoder := yaml.NewDecoder(bytes.NewBufferString(fmt.Sprintf("1%f%%", math.MaxFloat64)))

	err := decoder.Decode(&quota)
	require.ErrorIs(t, err, units.ErrInvalidQuotaFormat)
}

func TestCanEncodeToYamlForPercentages(t *testing.T) {
	t.Parallel()

	buf := strings.Builder{}
	encoder := yaml.NewEncoder(&buf)
	require.NoError(t, encoder.Encode(units.NewDiskQuotaInPercent(10)))
	require.Equal(t, "10%\n", buf.String())
}

func TestCanEncodeToYamlForBytes(t *testing.T) {
	t.Parallel()

	buf := strings.Builder{}
	encoder := yaml.NewEncoder(&buf)
	require.NoError(t, encoder.Encode(units.NewDiskQuotaInBytes(units.Bytes{10})))
	require.Equal(t, "10B\n", buf.String())
}

func TestCanGetBytesFromPercent(t *testing.T) {
	t.Parallel()

	quota := units.NewDiskQuotaInPercent(10)
	b, err := quota.Bytes(t.TempDir())
	require.NoError(t, err)
	require.GreaterOrEqual(t, b.Bytes, int64(10))
}

func TestCanGetBytesFromAbsolute(t *testing.T) {
	t.Parallel()

	quota := units.NewDiskQuotaInBytes(units.Bytes{10})
	b, err := quota.Bytes(t.TempDir())
	require.NoError(t, err)
	require.GreaterOrEqual(t, b.Bytes, int64(10))
}

func TestHandlesRequestingQuotaInPercentFromNonExistentMountpoint(t *testing.T) {
	t.Parallel()

	quota := units.NewDiskQuotaInPercent(10)
	_, err := quota.Bytes(t.TempDir() + "/nonexistent")
	require.ErrorIs(t, err, fs.ErrNotExist)
}
