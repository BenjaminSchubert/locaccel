package units_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/units"
)

func TestCanConvertFromYaml(t *testing.T) {
	t.Parallel()

	b := units.Bytes{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("10"))

	err := decoder.Decode(&b)
	require.NoError(t, err)
	require.Equal(t, units.Bytes{10}, b)
}

func TestCanDecode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		val      string
		expected units.Bytes
	}{
		{"1000", units.Bytes{1000}},
		{"1024B", units.Bytes{1024}},
		{"10KiB", units.Bytes{10240}},
		{"12MiB", units.Bytes{12582912}},
		{"12GiB", units.Bytes{12884901888}},
		{"1.5TiB", units.Bytes{1649267441664}},
		{"1.5 TiB", units.Bytes{1649267441664}},
		{"10KB", units.Bytes{10000}},
		{"12MB", units.Bytes{12000000}},
		{"12GB", units.Bytes{12000000000}},
		{"1.5TB", units.Bytes{1500000000000}},
		{"1.5 TB", units.Bytes{1500000000000}},
	} {
		t.Run(tc.val, func(t *testing.T) {
			t.Parallel()

			res, err := units.DecodeBytes(tc.val)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res)
		})
	}
}

func TestToString(t *testing.T) {
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

			require.Equal(t, tc.expected, units.Bytes{tc.value}.String())
		})
	}
}
