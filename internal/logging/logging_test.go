package logging

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLogFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		format string
		output string
	}{
		{"console", `^\x1b\[90m\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}((\+\d{2}:\d{2})|Z)\x1b\[0m \x1b\[32mINF\x1b\[0m \x1b\[1mlogging_test\.go:\d+\x1b\[0m\x1b\[36m >\x1b\[0m \x1b\[1mtest message\x1b\[0m\n$`},
		{"json", `^\{"level":"info","time":"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}((\+\d{2}:\d{2})|Z)","caller":.*\/logging_test\.go:\d+","message":"test message"\}\n$`},
	} {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()

			writer := bytes.NewBuffer(nil)
			logger, err := CreateLogger(zerolog.DebugLevel, tc.format, writer)
			require.NoError(t, err)

			logger.Info().Msg("test message")
			require.Regexp(t, tc.output, writer.String())
		})
	}
}

func TestCannotSetInvalidLogFormat(t *testing.T) {
	t.Parallel()

	writer := bytes.NewBuffer(nil)
	logger, err := CreateLogger(zerolog.DebugLevel, "invalid", writer)
	require.ErrorIs(t, err, ErrInvalidLogFormat)
	require.Equal(t, zerolog.Logger{}, logger)
}
