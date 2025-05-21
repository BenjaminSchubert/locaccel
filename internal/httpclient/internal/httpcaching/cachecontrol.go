// Implements the parsing for RFC 9111 section 5.2.2 'Response directives' in addition to RFC 8246 and 5861
//
//		See https://datatracker.ietf.org/doc/html/rfc9111
//		See https://datatracker.ietf.org/doc/html/rfc8246
//		See https://datatracker.ietf.org/doc/html/rfc5861
//	 See https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Cache-Control
package httpcaching

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var (
	ErrMissingArgument = errors.New("missing argument")
	ErrInvalidArgument = errors.New("invalid argument")
)

type CacheControlResponseDirective struct {
	Immutable            bool
	MaxAge               time.Duration
	MustRevalidate       bool
	MustUnderstand       bool
	NoCache              bool
	NoStore              bool
	NoTransform          bool
	Private              bool
	ProxyRevalidate      bool
	Public               bool
	SMaxAge              time.Duration
	StaleWhileRevalidate time.Duration
	StaleIfError         time.Duration
}

func ParseCacheControlDirective(
	header []string,
	logger *zerolog.Logger,
) (CacheControlResponseDirective, error) {
	response := CacheControlResponseDirective{}
	seen := make(map[string]struct{}, 0)

	for _, hdr := range header {
		for directive := range strings.SplitSeq(hdr, ",") {
			key, val, found := strings.Cut(strings.TrimSpace(directive), "=")
			if _, ok := seen[key]; ok {
				continue // Duplicate entry, only the first value is valid
			}
			seen[key] = struct{}{}

			if found {
				switch key {
				case "max-age":
					v, err := strconv.Atoi(val)
					if err != nil {
						return response, fmt.Errorf(
							"%w for directive 'max-age: %s",
							ErrInvalidArgument,
							err,
						)
					}
					response.MaxAge = time.Duration(v) * time.Second
				// no-cache can be qualified or unqualified
				// we only implement the unqualified version as it is simpler
				case "no-cache":
					logger.Trace().
						Str("directive", directive).
						Msg("a qualified version of 'no-cache' has been encountered")
					response.NoCache = true
				// private can be qualified or unqualified
				// we only implement the unqualified version as it is simpler
				case "private":
					logger.Trace().
						Str("directive", directive).
						Msg("a qualified version of 'private' has been encountered")
					response.Private = true
				case "s-maxage":
					v, err := strconv.Atoi(val)
					if err != nil {
						return response, fmt.Errorf(
							"%w for directive 's-maxage': %s",
							ErrInvalidArgument,
							err,
						)
					}
					response.SMaxAge = time.Duration(v) * time.Second
				case "stale-while-revalidate":
					v, err := strconv.Atoi(val)
					if err != nil {
						return response, fmt.Errorf(
							"%w for directive 'stale-while-revalidate': %s",
							ErrInvalidArgument,
							err,
						)
					}
					response.StaleWhileRevalidate = time.Duration(v) * time.Second
				case "stale-if-error":
					v, err := strconv.Atoi(val)
					if err != nil {
						return response, fmt.Errorf(
							"%w for directive 'stale-if-error': %s",
							ErrInvalidArgument,
							err,
						)
					}
					response.StaleIfError = time.Duration(v) * time.Second
				default:
					logger.Warn().
						Str("directive", directive).
						Msg("received an unknown directive in Cache-Control header")
				}
			} else {
				switch key {
				case "must-revalidate":
					response.MustRevalidate = true
				case "must-understand":
					response.MustUnderstand = true
				case "no-cache":
					response.NoCache = true
				case "no-store":
					response.NoStore = true
				case "no-transform":
					response.NoTransform = true
				case "private":
					response.Private = true
				case "proxy-revalidate":
					response.ProxyRevalidate = true
				case "public":
					response.Public = true
				case "immutable":
					response.Immutable = true
				default:
					logger.Warn().Str("directive", directive).Msg("received an unknown directive in Cache-Control header")
				}
			}
		}
	}

	return response, nil
}
