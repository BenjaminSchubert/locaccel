package admin

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

func RegisterHandler(handler *http.ServeMux, cache *httpclient.Cache) {
	templates := template.Must(template.New("index").ParseFS(templatesFS, "templates/*.tmpl"))

	handler.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		id, _ := hlog.IDFromRequest(r)

		stats, err := cache.GetStatistics(r.Context(), id.String())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("unable to gather statistics")
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("Unable to gather statistics")); err != nil {
				hlog.FromRequest(r).Panic().Err(err).Msg("error returning an anser")
			}

			return
		}

		err = templates.ExecuteTemplate(w, "index.html.tmpl", stats)
		if err != nil {
			hlog.FromRequest(r).Panic().Err(err).Msg("error sending the index.html")
		}
	})
}
