package goproxy

import (
	"net/http"
	"net/url"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(
	upstream string,
	sumdb string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	if sumdb[len(sumdb)-1] != '/' {
		sumdb += "/"
	}

	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}
	sumdbUpstreams := make([]*url.URL, 0, len(upstreamCaches))
	for _, uri := range upstreamCaches {
		u := *uri
		u.Path += "/sumdb/"
	}
	sumdbCaches := httpclient.UpstreamCache{Uris: sumdbUpstreams, Proxy: false}

	handler.HandleFunc("GET /sumdb/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			sumdb+r.URL.RequestURI()[7:],
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
