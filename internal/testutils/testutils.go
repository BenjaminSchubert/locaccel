package testutils

import (
	"net/http"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLogger(tb testing.TB) *zerolog.Logger {
	tb.Helper()

	logger := zerolog.New(zerolog.NewConsoleWriter(zerolog.ConsoleTestWriter(tb)))
	return &logger
}

type RequestCounterMiddleware struct {
	t     *testing.T
	calls map[string]int
	lock  sync.Mutex
}

func NewRequestCounterMiddleware(t *testing.T) *RequestCounterMiddleware {
	t.Helper()

	middleware := &RequestCounterMiddleware{t, make(map[string]int), sync.Mutex{}}
	t.Cleanup(middleware.validate)
	return middleware
}

func (m *RequestCounterMiddleware) GetHandler(next http.Handler, name string) http.Handler {
	m.t.Helper()
	m.calls[name] = 0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.lock.Lock()
		m.calls[name] += 1
		m.lock.Unlock()
		next.ServeHTTP(w, r)
	})
}

func (m *RequestCounterMiddleware) validate() {
	m.t.Helper()
	initialKey := ""
	initialVal := 0

	for key, val := range m.calls {
		if initialKey == "" {
			initialKey = key
			initialVal = val
			continue
		}
		require.Equal(m.t, initialVal, val, "comparing %s and %s", initialKey, key)
	}
}
