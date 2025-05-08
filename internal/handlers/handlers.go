package handlers

import (
	"net/http"

	"github.com/rs/zerolog/hlog"
)

func NotImplemented(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	if _, err := w.Write([]byte("Not implemented")); err != nil {
		hlog.FromRequest(r).Panic().Err(err).Msg("Error sending response to client")
	}
}
