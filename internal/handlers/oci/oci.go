package oci

import (
	"io"
	"net/http"

	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(registry string, handler *http.ServeMux, client *httpclient.Client) {
	handler.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		url := registry + r.URL.RequestURI()
		upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, url, r.Body)
		if err != nil {
			hlog.FromRequest(r).Panic().Err(err).Msg("Error generating new upstream request")
		}
		copyHeaders(r.Header, upstreamReq.Header)

		forward(client, upstreamReq, w, r)
	})
}

func forward(
	client *httpclient.Client,
	upstreamRequest *http.Request,
	w http.ResponseWriter,
	originalRequest *http.Request,
) {
	resp, err := client.Do(upstreamRequest)
	if err != nil {
		hlog.FromRequest(originalRequest).
			Panic().
			Err(err).
			Msg("Error forwarding request to upstream")
	}
	defer func() {
		if resp.Body.Close() != nil {
			hlog.FromRequest(originalRequest).
				Panic().
				Err(err).
				Msg("Error closing the body of the upstream request")
		}
	}()

	copyHeaders(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		hlog.FromRequest(originalRequest).Panic().Err(err).Msg("Error sending response to client")
	}
}

func copyHeaders(src, dest http.Header) {
	for key, vals := range src {
		dest[key] = vals
	}
}
