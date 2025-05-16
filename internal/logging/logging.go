package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var ErrInvalidLogFormat = errors.New("invalid log format requested")

func CreateLogger(level zerolog.Level, format string) (zerolog.Logger, error) {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	var w io.Writer
	switch format {
	case "console":
		w = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stderr
			w.TimeFormat = time.RFC3339
		})
	case "json":
		w = os.Stderr
	default:
		return zerolog.Logger{}, fmt.Errorf("%w: %s", ErrInvalidLogFormat, format)
	}

	return zerolog.New(w).Level(level).With().Timestamp().Caller().Logger(), nil
}
