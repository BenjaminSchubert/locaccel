// Implementation for RFC 9111: https://datatracker.ietf.org/doc/html/rfc9111
//
//		Useful links:
//	   - RFC 9110 (HTTP semantics): https://datatracker.ietf.org/doc/html/rfc9110
//			-
package httpcaching

import (
	"net/http"

	"github.com/rs/zerolog"
)

func IsCacheable(r *http.Response, logger *zerolog.Logger) bool {
	// Implements RFC 9111 section 3
	//
	// A cache MUST NOT store a response to a request unless:
	//
	// - the request method is understood by the cache;
	// - the response status code is final (see Section 15 of RFC 9110);
	// - if the response status code is 206 or 304, or the must-understand cache directive (see Section 5.2.2.3) is present: the cache understands the response status code;
	// - the no-store cache directive is not present in the response (see Section 5.2.2.5);
	// - if the cache is shared: the private response directive is either not present or allows a shared cache to store a modified response; see Section 5.2.2.7);
	// - if the cache is shared: the Authorization header field is not present in the request (see Section 11.6.2 of RFC 9110) or a response directive is present that explicitly allows shared caching (see Section 3.5); and
	// - the response contains at least one of the following:
	//   - a public response directive (see Section 5.2.2.9);
	//   - a private response directive, if the cache is not shared (see Section 5.2.2.7);
	// 	 - an Expires header field (see Section 5.3);
	//   - a max-age response directive (see Section 5.2.2.1);
	//   - if the cache is shared: an s-maxage response directive (see Section 5.2.2.10);
	//   - a cache extension that allows it to be cached (see Section 5.2.3);
	//   - or a status code that is defined as heuristically cacheable (see Section 4.2.2).
	//
	// Adding to this section 3.3:
	//
	//  If the request method is GET, the response status code is 200 (OK), and
	//  the entire response header section has been received, a cache MAY store
	// a response that is not complete (Section 6.1 of [HTTP]) provided that the
	//  stored response is recorded as being incomplete. Likewise, a
	// 206 (Partial Content) response MAY be stored as if it were an incomplete
	//  200 (OK) response. However, a cache MUST NOT store incomplete or
	// partial-content responses if it does not support the Range and
	// Content-Range header fields or if it does not understand the range units
	// used in those fields.

	// Reasons it cannot be cached
	// FIXME: can we cache other status codes?
	if r.StatusCode != http.StatusOK {
		return false
	}

	cacheControl, err := ParseCacheControlDirective(r.Header.Values("Cache-Control"), logger)
	if err != nil {
		logger.Warn().Err(err).Msg("unable to parse cache control directive")
		return false
	}

	if cacheControl.NoStore || cacheControl.Private {
		return false
	}

	if _, ok := r.Header["Authorization"]; ok {
		if !cacheControl.MustRevalidate && !cacheControl.Public && cacheControl.SMaxAge == 0 {
			return false
		}
	}

	if _, ok := r.Header["Range"]; ok {
		return false
	}
	if _, ok := r.Header["Content-Range"]; ok {
		return false
	}

	// Reasons it could be cached
	if cacheControl.Public || cacheControl.MaxAge != 0 || cacheControl.SMaxAge != 0 {
		return true
	}

	if _, ok := r.Header["Expires"]; ok {
		return true
	}

	// If the response has a Last-Modified header field (Section 8.8.2 of RFC 9110),
	// caches are encouraged to use a heuristic expiration value that is no more
	// than some fraction of the interval since that time. A typical setting of
	// this fraction might be 10%.
	if val := r.Header.Get("Last-Modified"); val != "" {
		if _, err = http.ParseTime(val); err == nil {
			return true
		}
		logger.Warn().Err(err).Msg("unable to parse Last-Modified header")
	}

	// Be safe, don't cache otherwise
	return false
}
