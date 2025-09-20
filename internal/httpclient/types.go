package httpclient

import (
	"net/http"
	"time"
)

//go:generate go tool github.com/tinylib/msgp -io=false
//msgp:replace http.Header with:map[string][]string
//msgp:tuple CachedResponse

type CachedResponse struct {
	ContentHash            string
	StatusCode             int
	Headers                http.Header
	VaryHeaders            http.Header
	TimeAtResponseCreation time.Time
}

type CachedResponses []CachedResponse
