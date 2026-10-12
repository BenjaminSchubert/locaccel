package testutils

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type LogRecordHook struct {
	ErrorMessages []string
}

func (l *LogRecordHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level >= zerolog.ErrorLevel {
		l.ErrorMessages = append(l.ErrorMessages, msg)
	}
}

func TestLogger(tb testing.TB, expectedErrors []string) *zerolog.Logger {
	tb.Helper()

	logRecorder := LogRecordHook{}
	tb.Cleanup(func() {
		require.Equal(tb, expectedErrors, logRecorder.ErrorMessages)
	})

	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(tb))).
		Hook(&logRecorder)
	return &logger
}
