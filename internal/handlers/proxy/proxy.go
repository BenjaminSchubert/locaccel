package proxy

import (
	"net/http"

	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func RegisterHandler(
	allowedHostnames []string,
	handler *http.ServeMux,
	client *httpclient.Client,
) {
	hostnames := make(map[string]struct{}, len(allowedHostnames))
	for _, hostname := range allowedHostnames {
		hostnames[hostname] = struct{}{}
	}

	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := hostnames[r.Host]; !ok {
			w.WriteHeader(http.StatusForbidden)
			if _, err := w.Write([]byte("The server cannot authorize proxying to the requested upstream")); err != nil {
				hlog.FromRequest(r).
					Panic().
					Err(err).
					Msg("An error happened sending the request back to the client")
			}

			return
		}

		handlers.Forward(w, r, r.URL.String(), client, nil)
	})
}
