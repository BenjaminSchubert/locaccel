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

func getVaryHeaderNames(headers http.Header) []string {
	hdrs := make([]string, 0)

	varys := headers["Vary"]
	for _, vary := range varys {
		for field := range strings.SplitSeq(vary, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				hdrs = append(hdrs, field)
			}
		}
	}

	return hdrs
}

func normalizeVaryHeaders(headerVal []string) []string {
	if headerVal == nil {
		return nil
	}
	return []string{strings.Join(headerVal, ", ")}
}

func ExtractVaryHeaders(reqHeaders, respHeaders http.Header) http.Header {
	varyHeaders := getVaryHeaderNames(respHeaders)
	relevantHeaders := http.Header{}

	for _, header := range varyHeaders {
		relevantHeaders[header] = normalizeVaryHeaders(reqHeaders[header])
	}

	return relevantHeaders
}

func MatchVaryHeaders(reqHeaders, varyHeaders http.Header, logger *zerolog.Logger) bool {
	if len(varyHeaders) == 0 {
		return true
	}

	if _, ok := varyHeaders["*"]; ok {
		return false
	}

	for headerName, headerValue := range varyHeaders {
		reqHeader := normalizeVaryHeaders(reqHeaders[headerName])
		if len(reqHeader) != len(headerValue) {
			logger.Debug().
				Str("header", headerName).
				Strs("currentHeaders", reqHeader).
				Strs("originalHeaders", headerValue).
				Msg("unable to reuse query without validating due to difference in header length")
			return false
		}

		if len(reqHeader) == 1 && headerValue[0] != reqHeader[0] {
			logger.Debug().
				Str("header", headerName).
				Strs("currentHeaders", reqHeader).
				Strs("originalHeaders", headerValue).
				Msg("unable to reuse query without validating due to different headers")
			return false
		}
	}

	return true
}
