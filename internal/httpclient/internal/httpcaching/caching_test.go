package httpcaching_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/benjaminschubert/locaccel/internal/httpclient/internal/httpcaching"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func TestResponseIsCacheable(t *testing.T) {
	t.Parallel()

	for _, private := range []bool{true, false} {
		for _, tc := range []struct {
			description string
			StatusCode  int
			headers     http.Header
			expected    bool
		}{
			{"invalid-status-code", http.StatusNotFound, nil, false},
			{"invalid-cache-control-header", http.StatusOK, http.Header{"Cache-Control": []string{"max-age=hello"}}, false},
			{"no-store", http.StatusOK, http.Header{"Cache-Control": []string{"no-store"}}, false},
			{"private", http.StatusOK, http.Header{"Cache-Control": []string{"private"}}, private},
			{"authenticated-no-cache-control", http.StatusOK, http.Header{"Authorization": []string{}}, false},
			{"authenticated", http.StatusOK, http.Header{"Authorization": []string{}, "Cache-Control": []string{"max-age=10"}}, private},
			{"authenticated-public", http.StatusOK, http.Header{"Authorization": []string{""}, "Cache-Control": []string{"public"}}, true},
			{"range", http.StatusOK, http.Header{"Range": []string{"123"}}, false},
			{"content-range", http.StatusOK, http.Header{"Content-Range": []string{"123"}}, false},
			{"expires", http.StatusOK, http.Header{"Expires": []string{"123"}}, true},
			{"last-modified", http.StatusOK, http.Header{"Last-Modified": []string{"Fri, 15 Dec 2023 11:01:18 GMT"}}, true},
			{"last-modified-invalid", http.StatusOK, http.Header{"Last-Modified": []string{"Wrong date"}}, false},
			{"etag", http.StatusOK, http.Header{"Etag": []string{"one"}}, true},
			{"no-information", http.StatusOK, nil, false},
		} {
			t.Run(fmt.Sprintf("%s [%t]", tc.description, private), func(t *testing.T) {
				t.Parallel()

				resp := http.Response{StatusCode: tc.StatusCode, Header: tc.headers}
				assert.Equal(
					t,
					tc.expected,
					httpcaching.IsCacheable(&resp, private, testutils.TestLogger(t)),
				)
			})
		}
	}
}
