package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestLogger(t *testing.T) zerolog.Logger {
	return zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(t)))
}
