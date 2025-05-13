package testutils

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestLogger(t *testing.T) *zerolog.Logger {
	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(t)))
	return &logger
}
