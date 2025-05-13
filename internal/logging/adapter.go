package logging

import (
	"strings"

	"github.com/rs/zerolog"
)

type LoggerAdapter struct{ log *zerolog.Logger }

func NewLoggerAdapter(log *zerolog.Logger) *LoggerAdapter {
	return &LoggerAdapter{log}
}

func (l *LoggerAdapter) Debugf(format string, args ...any) {
	l.log.Debug().Msgf(strings.TrimSpace(format), args...)
}

func (l *LoggerAdapter) Infof(format string, args ...any) {
	l.log.Info().Msgf(strings.TrimSpace(format), args...)
}

func (l *LoggerAdapter) Warningf(format string, args ...any) {
	l.log.Warn().Msgf(strings.TrimSpace(format), args...)
}

func (l *LoggerAdapter) Errorf(format string, args ...any) {
	l.log.Error().Msgf(strings.TrimSpace(format), args...)
}
