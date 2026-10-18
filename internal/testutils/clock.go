package testutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type Clock struct {
	current time.Time
}

func NewClock(t *testing.T) *Clock {
	t.Helper()

	testTime, err := time.Parse(time.RFC3339, "2024-01-01T12:13:14Z")
	require.NoError(t, err)
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
