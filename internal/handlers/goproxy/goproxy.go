package goproxy

import (
	"net/http"
	"net/url"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(
	upstream string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}
	sumdbCaches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}
	for _, uri := range sumdbCaches.Uris {
		uri.Path += "/sumdb/"
	}

	handler.HandleFunc("GET /sumdb/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			"https://sum.golang.org/"+r.URL.RequestURI()[7:],
			client,
			func(body []byte, resp *http.Response) ([]byte, error) {
				return body, nil
			},
			sumdbCaches,
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
			caches,
		)
	})
}
