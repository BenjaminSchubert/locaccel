package testutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type Clock struct {
	current time.Time
}

func NewClock(tb testing.TB) *Clock {
	tb.Helper()

	testTime, err := time.Parse(time.RFC3339, "2024-01-01T12:13:14Z")
	require.NoError(tb, err)
	return &Clock{testTime}
}

func (c *Clock) Since(t time.Time) time.Duration {
	return c.current.Sub(t)
}

func (c *Clock) Now() time.Time {
	return c.current
}

func (c *Clock) Advance() {
	c.current = c.current.Add(time.Second)
}
