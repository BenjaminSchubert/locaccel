package goproxy

import (
	"net/http"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(upstream string, handler *http.ServeMux, client *httpclient.Client) {
	handler.HandleFunc("GET /sumdb/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			"https://sum.golang.org/"+r.URL.RequestURI()[7:],
			client,
			func(body []byte, resp *http.Response) ([]byte, error) {
				return body, nil
			},
		)
	})

	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			upstream+r.URL.RequestURI(),
			client,
			func(body []byte, resp *http.Response) ([]byte, error) {
				return body, nil
			},
		)
	})
}
