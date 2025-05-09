// Implements section 4.2 of RFC 9111 'Freshness'
//
// See https://datatracker.ietf.org/doc/html/rfc9111#section-4.2
package httpcaching

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

func getFreshnessLifetime(headers http.Header, cacheControl CacheControlResponseDirective, logger zerolog.Logger) time.Duration {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.1
	if cacheControl.SMaxAge != 0 {
		return cacheControl.SMaxAge
	}
	if cacheControl.MaxAge != 0 {
		return cacheControl.MaxAge
	}

	if expires := headers.Get("Expires"); expires != "" {
		expiry, err := http.ParseTime(expires)
		if err != nil {
			logger.Warn().Err(err).Msg("Expires header is in an invalid format")
		} else {
			date, err := http.ParseTime(headers.Get("Date"))
			if err != nil {
				logger.Error().Err(err).Msg("BUG: Date header is in an invalid format, which should not happen")
			}
			return expiry.Sub(date)
		}
	}

	// FIXME: implement https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.2
	return 0
}

func getCurrentAge(
	headers http.Header,
	requestTime, responseTime time.Time,
	logger zerolog.Logger,
) time.Duration {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.3
	// FIXME: can we precompute most of this and avoid storing this additional data?

	ageStr := headers.Get("Age")
	age := 0

	if ageStr != "" {
		var err error
		age, err = strconv.Atoi(headers.Get("Age"))
		if err != nil {
			logger.Error().
				Err(err).
				Str("age", headers.Get("Age")).
				Msg("response has an invalid Age header")
			age = 0
		}
	}

	date, err := http.ParseTime(headers.Get("Date"))
	if err != nil {
		logger.Error().
			Err(err).
			Msg("BUG: Date header is in an invalid format, which should not happen")
		date = time.Time{}
	}

	apparentAge := max(0, responseTime.Sub(date))
	responseDelay := responseTime.Sub(requestTime)
	correctedAgeValue := (time.Second * time.Duration(age)) + responseDelay

	correctedInitialAge := max(apparentAge, correctedAgeValue)

	residentTime := time.Since(responseTime)

	return (correctedInitialAge + residentTime).Truncate(time.Second)
}

func IsFresh(
	headers http.Header,
	requestTime, responseTime time.Time,
	logger zerolog.Logger,
) (time.Duration, bool) {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2
	cacheControl, err := ParseCacheControlDirective(headers.Values("cache-control"))
	if err != nil {
		logger.Warn().Err(err).Msg("unable to parse cache control directives")
	}
	age := getCurrentAge(headers, requestTime, responseTime, logger)

	return age, cacheControl.NoCache || cacheControl.MustRevalidate || getFreshnessLifetime(headers, cacheControl, logger) > age
}
