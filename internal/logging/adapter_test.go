package logging

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLogAdapter(t *testing.T) {
	t.Parallel()

	writer := bytes.NewBuffer(nil)
	zlogger, err := CreateLogger(zerolog.TraceLevel, "json", writer)
	require.NoError(t, err)

	logger := NewLoggerAdapter(&zlogger)

	logger.Debugf("test %s", "debug")
	require.Contains(t, writer.String(), `"level":"debug"`)
	require.Contains(t, writer.String(), `"message":"test debug"`)
	writer.Reset()

	logger.Infof("test %s", "info")
	require.Contains(t, writer.String(), `"level":"info"`)
	require.Contains(t, writer.String(), `"message":"test info"`)
	writer.Reset()

	logger.Warningf("test %s", "warning")
	require.Contains(t, writer.String(), `"level":"warn"`)
	require.Contains(t, writer.String(), `"message":"test warning"`)
	writer.Reset()

	logger.Errorf("test %s", "error")
	require.Contains(t, writer.String(), `"level":"error"`)
	require.Contains(t, writer.String(), `"message":"test error"`)
}
