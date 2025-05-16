package handlers

import (
	"net/http"
	"net/http/pprof"

	"github.com/rs/zerolog/hlog"
)

func NotImplemented(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	if _, err := w.Write([]byte("Not implemented")); err != nil {
		hlog.FromRequest(r).Panic().Err(err).Msg("Error sending response to client")
	}
}

func RegisterProfilingHandlers(handler *http.ServeMux, prefix string) {
	handler.HandleFunc(prefix+"", pprof.Index)
	handler.Handle(prefix+"allocs", pprof.Handler("allocs"))
	handler.Handle(prefix+"block", pprof.Handler("block"))
	handler.HandleFunc(prefix+"cmdline", pprof.Cmdline)
	handler.Handle(prefix+"goroutine", pprof.Handler("goroutine"))
	handler.Handle(prefix+"heap", pprof.Handler("heap"))
	handler.HandleFunc(prefix+"profile", pprof.Profile)
	handler.HandleFunc(prefix+"symbol", pprof.Symbol)
	handler.Handle(prefix+"threadcreate", pprof.Handler("threadcreate"))
	handler.HandleFunc(prefix+"trace", pprof.Trace)
}
