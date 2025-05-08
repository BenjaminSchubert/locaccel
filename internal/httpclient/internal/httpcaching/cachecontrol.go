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

func ParseCacheControlDirective(header []string) (CacheControlResponseDirective, error) {
	response := CacheControlResponseDirective{}

	for _, hdr := range header {
		for _, directive := range strings.Split(hdr, ",") {
			key, val, found := strings.Cut(strings.TrimSpace(directive), "=")

			if found {
				switch key {
				case "max-age":
					// FIXME: only the first value should be used if it's mentioned multiple times
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
				// FIXME: add a tracelog if encountering no-cache with qualified
				case "no-cache":
					response.NoCache = true
				// private can be qualified or unqualified
				// we only implement the unqualified version as it is simpler
				// FIXME: add a tracelog if encountering private with qualified
				case "private":
					response.Private = true
				case "s-max-age":
					// FIXME: only the first value should be used if it's mentioned multiple times
					v, err := strconv.Atoi(val)
					if err != nil {
						return response, fmt.Errorf(
							"%w for directive 's-max-age': %s",
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
					// FIXME: log
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
					// FIXME: log
				}
			}
		}
	}

	return response, nil
}
