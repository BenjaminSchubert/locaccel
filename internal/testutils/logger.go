package testutils

import (
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type LogRecordHook struct {
	ErrorMessages []string
	mu            sync.Mutex
}

func (l *LogRecordHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level >= zerolog.ErrorLevel {
		l.mu.Lock()
		l.ErrorMessages = append(l.ErrorMessages, msg)
		l.mu.Unlock()
	}
}

func TestLogger(t *testing.T, expectedErrors []string) *zerolog.Logger {
	t.Helper()

	logRecorder := LogRecordHook{}
	t.Cleanup(func() {
		require.Equal(t, expectedErrors, logRecorder.ErrorMessages)
	})

	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(t))).
		Hook(&logRecorder)
	return &logger
}

func BenchLogger(b *testing.B) *zerolog.Logger {
	b.Helper()

	logger := zerolog.New(zerolog.Nop()).Level(zerolog.Disabled)
	return &logger
}
