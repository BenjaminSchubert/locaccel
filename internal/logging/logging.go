package logging

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var ErrInvalidLogFormat = errors.New("invalid log format requested")

func init() {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
}

func CreateLogger(level zerolog.Level, format string, dest io.Writer) (zerolog.Logger, error) {
	var w io.Writer
	switch format {
	case "console":
		w = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = dest
			w.TimeFormat = time.RFC3339
		})
	case "json":
		w = dest
	default:
		return zerolog.Logger{}, fmt.Errorf("%w: %s", ErrInvalidLogFormat, format)
	}

	return zerolog.New(w).Level(level).With().Timestamp().Caller().Logger(), nil
}
