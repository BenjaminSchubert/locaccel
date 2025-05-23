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

func getFreshnessLifetime(
	headers http.Header,
	cacheControl CacheControlResponseDirective,
	logger *zerolog.Logger,
) time.Duration {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.1
	// and https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.2

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

	if lastModified := headers.Get("Last-Modified"); lastModified != "" {
		modified, err := http.ParseTime(lastModified)
		if err != nil {
			logger.Warn().Err(err).Msg("Last-Modified header is in an invalid format")
		} else {
			date, err := http.ParseTime(headers.Get("Date"))
			if err == nil {
				return date.Sub(modified) / 10
			}
			logger.Error().Err(err).Msg("BUG: Date header is in an invalid format, which should not happen")

		}
	}
	return 0
}

func GetEstimatedResponseCreation(
	headers http.Header,
	requestTime, responseTime time.Time,
	logger *zerolog.Logger,
) time.Time {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.3
	// This initial part computes the approximate date at which the response was
	// actually created
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

	return responseTime.Add(-max(apparentAge, correctedAgeValue))
}

func GetCurrentAge(responseCreationTime time.Time) time.Duration {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2.3
	// The responseCreationTime expects to be computed from the
	// GetEstimatedResponseCreation function
	return time.Since(responseCreationTime).Truncate(time.Second)
}

func IsFresh(
	headers http.Header,
	cacheControl CacheControlResponseDirective,
	responseCreationTime time.Time,
	logger *zerolog.Logger,
) (time.Duration, bool) {
	// Implements https://datatracker.ietf.org/doc/html/rfc9111#section-4.2
	age := GetCurrentAge(responseCreationTime)
	return age, getFreshnessLifetime(headers, cacheControl, logger) > age
}
