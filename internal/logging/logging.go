package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

func CreateLogger(level zerolog.Level) zerolog.Logger {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// FIXME: disable console write in production
	return zerolog.New(
		zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stderr
			w.TimeFormat = time.RFC3339
		})).Level(level).With().Timestamp().Caller().Logger()
}
