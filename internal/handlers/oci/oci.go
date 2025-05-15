package oci

import (
	"net/http"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(registry string, handler *http.ServeMux, client *httpclient.Client) {
	handler.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(w, r, registry+r.URL.RequestURI(), client, nil)
	})
}
