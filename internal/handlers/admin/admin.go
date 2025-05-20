package admin

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/rs/zerolog/hlog"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

type data struct {
	Stats httpclient.CacheStatistics
	Conf  string
}

func RegisterHandler(handler *http.ServeMux, cache *httpclient.Cache, conf *config.Config) error {
	templates, err := template.New("index").ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return err
	}

	renderedConfig, err := renderConfig(conf)
	if err != nil {
		return err
	}

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

		err = templates.ExecuteTemplate(w, "index.html.tmpl", data{stats, renderedConfig})
		if err != nil {
			hlog.FromRequest(r).Panic().Err(err).Msg("error sending the index.html")
		}
	})

	return nil
}

func renderConfig(conf *config.Config) (string, error) {
	buffer := strings.Builder{}
	encoder := yaml.NewEncoder(&buffer)
	err := encoder.Encode(conf)
	return buffer.String(), err
}
