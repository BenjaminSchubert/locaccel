// This implements the logic related to RFC 9110 'Vary' headers and RFC 9111 section 4.1 for their use
//
// See https://datatracker.ietf.org/doc/html/rfc9110#section-12.5.5
// See https://datatracker.ietf.org/doc/html/rfc9111#section-4.1 for usage
package httpcaching

import (
	"net/http"
	"strings"

	"github.com/rs/zerolog"
)

func normalizeVaryHeaders(headers http.Header) map[string]struct{} {
	hdrs := make(map[string]struct{})

	varys := headers["Vary"]
	for _, vary := range varys {
		for field := range strings.SplitSeq(vary, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				hdrs[field] = struct{}{}
			}
		}
	}

	return hdrs
}

func ExtractVaryHeaders(reqHeaders, respHeaders http.Header) http.Header {
	// FIXME: we should normalize known headers
	varyHeaders := normalizeVaryHeaders(respHeaders)
	relevantHeaders := http.Header{}

	for header := range varyHeaders {
		relevantHeaders[header] = reqHeaders[header]
	}

	return relevantHeaders
}

func MatchVaryHeaders(reqHeaders, varyHeaders http.Header, logger zerolog.Logger) bool {
	if _, ok := varyHeaders["*"]; ok {
		return false
	}

	// FIXME: we should normalize known headers
	for headerName, headerValue := range varyHeaders {
		reqHeader := reqHeaders[headerName]
		if len(reqHeader) != len(headerValue) {
			logger.Debug().
				Str("header", headerName).
				Strs("currentHeaders", reqHeader).
				Strs("originalHeaders", headerValue).
				Msg("unable to reuse query without validating due to difference in header length")
			return false
		}

		for i, val := range headerValue {
			if val != reqHeader[i] {
				logger.Debug().
					Str("header", headerName).
					Strs("currentHeaders", reqHeader).
					Strs("originalHeaders", headerValue).
					Msg("unable to reuse query without validating due to different headers")
				return false
			}
		}
	}

	return true
}
