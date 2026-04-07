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
			explicit    bool
		}{
			{"invalid-status-code", http.StatusNotFound, nil, false, true},
			{"invalid-cache-control-header", http.StatusOK, http.Header{"Cache-Control": []string{"max-age=hello"}}, false, true},
			{"no-store", http.StatusOK, http.Header{"Cache-Control": []string{"no-store"}}, false, true},
			{"private", http.StatusOK, http.Header{"Cache-Control": []string{"private"}}, private, true},
			{"authenticated-no-cache-control", http.StatusOK, http.Header{"Authorization": []string{}}, false, !private},
			{"authenticated", http.StatusOK, http.Header{"Authorization": []string{}, "Cache-Control": []string{"max-age=10"}}, private, true},
			{"authenticated-public", http.StatusOK, http.Header{"Authorization": []string{""}, "Cache-Control": []string{"public"}}, true, true},
			{"range", http.StatusOK, http.Header{"Range": []string{"123"}}, false, true},
			{"content-range", http.StatusOK, http.Header{"Content-Range": []string{"123"}}, false, true},
			{"expires", http.StatusOK, http.Header{"Expires": []string{"123"}}, true, true},
			{"last-modified", http.StatusOK, http.Header{"Last-Modified": []string{"Fri, 15 Dec 2023 11:01:18 GMT"}}, true, true},
			{"last-modified-invalid", http.StatusOK, http.Header{"Last-Modified": []string{"Wrong date"}}, false, false},
			{"etag", http.StatusOK, http.Header{"Etag": []string{"one"}}, true, true},
			{"no-information", http.StatusOK, nil, false, false},
		} {
			t.Run(fmt.Sprintf("%s private=%t", tc.description, private), func(t *testing.T) {
				t.Parallel()

				resp := http.Response{StatusCode: tc.StatusCode, Header: tc.headers}

				isCacheable, explicit := httpcaching.IsCacheable(
					&resp,
					private,
					testutils.TestLogger(t),
				)
				assert.Equal(t, tc.expected, isCacheable)
				assert.Equal(t, tc.explicit, explicit)
			})
		}
	}
}
