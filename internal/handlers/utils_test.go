package handlers_test

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var errTest = errors.New("test error")

func testEndpoint(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Etag", "123456")
			_, err := w.Write([]byte("Hello!"))
			assert.NoError(t, err)
		}),
	)
	t.Cleanup(srv.Close)
	return srv.URL
}

func testRequest(tb testing.TB) *http.Request {
	tb.Helper()

	testRequest, err := http.NewRequestWithContext(
		tb.Context(),
		http.MethodGet,
		"http://localhost.test/",
		nil,
	)
	require.NoError(tb, err)
	return testRequest
}

func TestCanForwardBasicQuery(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		testEndpoint(t),
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		nil,
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "Hello!", string(body))
}

func TestReturnsNotModifiedIfMatchesOriginalQuery(t *testing.T) {
	t.Parallel()
	testReq := testRequest(t)
	testReq.Header.Add("If-None-Match", "123456")

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testReq,
		testEndpoint(t),
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
			_, err := jsonHandler.Buffer.WriteString("Hi!")
			require.NoError(t, err)
			return nil
		},
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusNotModified, result.StatusCode)
	assert.Empty(t, string(body))
}

func TestReturnsBadGatewayWhenUnableToContactUpstream(t *testing.T) {
	t.Parallel()

	testReq := testRequest(t)
	testReq.Header.Add("If-None-Match", "123456")
	testReq.Header.Add("If-None-Match", "123456")

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testReq,
		"http://localhost.test",
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
			_, err := jsonHandler.Buffer.WriteString("Hi!")
			require.NoError(t, err)
			return nil
		},
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusBadGateway, result.StatusCode)
	assert.Empty(t, string(body))
}

func TestReturnsErrorOnTimeoutFromUpstream(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
			_, err := w.Write([]byte("Hello!"))
			assert.NoError(t, err)
		}),
	)
	t.Cleanup(srv.Close)

	httpClient, underlyingClient := testutils.NewClientWithUnderlyingClient(
		t, false, func(r *http.Request, s string) {}, testutils.TestLogger(t),
	)
	underlyingClient.Timeout = 5 * time.Millisecond

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		srv.URL,
		httpClient,
		nil,
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusGatewayTimeout, result.StatusCode)
	assert.Empty(t, string(body))
}

func TestCanModifyQueryBody(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		testEndpoint(t),
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
			_, err := jsonHandler.Buffer.WriteString("Hi!")
			require.NoError(t, err)
			return nil
		},
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "Hi!", string(body))
}

func TestReturnProperErrorWhenUnableToModify(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		testEndpoint(t),
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
			_, err := jsonHandler.Buffer.WriteString("Hi!")
			require.NoError(t, err)
			return errTest
		},
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
	assert.Empty(t, string(body))
}

func TestCanRecoverFromUpstreamError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		"http://localhost.test/",
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		nil,
		func(w http.ResponseWriter, err error) error {
			w.WriteHeader(http.StatusOK)
			_, err2 := w.Write([]byte("well, things happened"))
			return err2
		},
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "well, things happened", string(body))
}

func TestReportsErrorsFromAFailedRecovery(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	handlers.Forward(
		recorder,
		testRequest(t),
		"http://localhost.test/",
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		nil,
		func(w http.ResponseWriter, err error) error {
			return errTest
		},
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusBadGateway, result.StatusCode)
	assert.Empty(t, string(body))
}

func TestHandledModifyingGzippedRequestsTransparently(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)

			gw := gzip.NewWriter(w)
			_, err := gw.Write([]byte("Hello!"))
			assert.NoError(t, err)
			assert.NoError(t, gw.Close())
		}),
	)
	t.Cleanup(srv.Close)

	recorder := httptest.NewRecorder()

	req := testRequest(t)
	req.Header.Add("Accept-Encoding", "gzip")

	handlers.Forward(
		recorder,
		req,
		srv.URL,
		testutils.NewClientWithNotify(
			t,
			false,
			func(r *http.Request, s string) {},
			testutils.TestLogger(t),
		),
		func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
			assert.Equal(t, "Hello!", string(body))
			jsonHandler.Buffer.Write(body)
			return nil
		},
		nil,
		httpclient.UpstreamCache{},
	)

	result := recorder.Result()
	gr, err := gzip.NewReader(result.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(gr)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "Hello!", string(body))
}

func BenchmarkHandlingOfGzipResponses(b *testing.B) {
	for _, size := range []units.Bytes{{Bytes: 1024 * 16}, {Bytes: 1024 * 1024}, {Bytes: 1024 * 1024 * 16}, {Bytes: 1024 * 1024 * 100}} {
		b.Run("size="+size.String(), func(b *testing.B) {
			uncompressed := make([]byte, size.Bytes)
			_, err := rand.Read(uncompressed)
			require.NoError(b, err)

			buf := bytes.NewBuffer(nil)
			writer := gzip.NewWriter(buf)

			_, err = writer.Write(uncompressed)
			require.NoError(b, err)
			require.NoError(b, writer.Close())

			data := buf.Bytes()

			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Add("Content-Encoding", "gzip")
					w.Header().Add("Cache-Control", "no-store")
					w.WriteHeader(http.StatusOK)
					_, err := w.Write(data)
					assert.NoError(b, err)
				}),
			)
			b.Cleanup(srv.Close)

			client := testutils.NewClientWithNotify(
				b,
				false,
				func(r *http.Request, s string) {},
				testutils.TestLogger(b),
			)

			for b.Loop() {
				recorder := httptest.NewRecorder()

				req := testRequest(b)
				req.Header.Add("Accept-Encoding", "gzip")

				handlers.Forward(
					recorder,
					req,
					srv.URL,
					client,
					func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
						jsonHandler.Buffer.Write(body)
						return nil
					},
					nil,
					httpclient.UpstreamCache{},
				)

				result := recorder.Result()
				assert.Equal(b, http.StatusOK, result.StatusCode)

				_, err = io.Copy(io.Discard, result.Body)
				require.NoError(b, err)
				require.NoError(b, result.Body.Close())
			}
		})
	}
}
