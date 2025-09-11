package oci

import (
	"net/http"
	"net/url"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(
	registry string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}

	handler.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(w, r, registry+r.URL.RequestURI(), client, nil, caches)
	})
}
