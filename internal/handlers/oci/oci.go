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

	handler.HandleFunc("GET /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("path") == "" {
			handlers.Forward(w, r, registry+r.URL.RequestURI(), client, nil,
				func(w http.ResponseWriter, err error) error {
					w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Content-Length", "0")

					w.WriteHeader(http.StatusOK)
					return nil
				}, caches)
		} else {
			handlers.Forward(w, r, registry+r.URL.RequestURI(), client, nil, nil, caches)
		}
	})
}
