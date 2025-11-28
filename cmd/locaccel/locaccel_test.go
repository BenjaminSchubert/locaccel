package main

import (
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanGetVersion(t *testing.T) {
	t.Parallel()

	require.Equal(t, "(devel)", getVersion())
}

func TestCanLoadSpecifiedConfig(t *testing.T) {
	t.Parallel()

	conf := t.TempDir() + "/locaccel.yml"
	fp, err := os.Create(conf) //nolint:gosec
	require.NoError(t, err)

	_, err = fp.WriteString("host: 1.1.1.1\nlog:\n  level: debug")
	require.NoError(t, err)
	require.NoError(t, fp.Close())

	c, configNotExist, err := loadConfig(func(s string) (string, bool) {
		switch s {
		case "LOCACCEL_CONFIG_PATH":
			return conf, true
		default:
			return "", false
		}
	})

	require.NoError(t, err)
	assert.False(t, configNotExist)
	assert.Equal(t, "1.1.1.1", c.Host)
}

func TestFailsIfSpecifiedConfigDoesNotExist(t *testing.T) {
	t.Parallel()

	_, _, err := loadConfig(func(s string) (string, bool) {
		switch s {
		case "LOCACCEL_CONFIG_PATH":
			return t.TempDir() + "/locaccel.yml", true
		default:
			return "", false
		}
	})
	require.ErrorIs(t, err, fs.ErrNotExist)
}
