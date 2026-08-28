package admin

import (
	"embed"
	"errors"
	"html/template"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/hlog"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	"github.com/benjaminschubert/locaccel/internal/units"
	"github.com/benjaminschubert/locaccel/internal/version"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

type indexData struct {
	Version         string
	CacheStats      httpclient.CacheStatistics
	MiddlewareStats *middleware.Statistics
	Conf            string
}

type hostnameData struct {
	Hostname string
	Entries  httpclient.CacheList
}

func RegisterHandler(
	handler *http.ServeMux,
	cache *httpclient.Cache,
	conf *config.Config,
	middlewareStats *middleware.Statistics,
) error {
	funcs := template.FuncMap{
		"asBytes":   units.PrettyBytes[uint64],
		"join":      strings.Join,
		"uriEscape": url.QueryEscape,
	}
	templates, err := template.New("index").Funcs(funcs).ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return err
	}

	renderedConfig, err := renderConfig(conf)
	if err != nil {
		return err
	}

	handler.HandleFunc("GET /healthcheck", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("healthy")); err != nil {
			hlog.FromRequest(r).Panic().Err(err).Msg("error returning an answer")
		}
	})

	handler.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	handler.HandleFunc("GET /hostname/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := hlog.IDFromRequest(r)
		hostname := r.PathValue("hostname")

		list, err := cache.List(r.Context(), hostname, id.String())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("unable to list cached entries for hostname")
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("Unable to gather information")); err != nil {
				hlog.FromRequest(r).Panic().Err(err).Msg("error returning an answer")
			}
			return
		}

		err = templates.ExecuteTemplate(w, "list.html.tmpl", hostnameData{hostname, list})
		if err != nil {
			hlog.FromRequest(r).Panic().Err(err).Msg("error sending the list.html")
		}
	})

	handler.HandleFunc("DELETE /cache/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		logger := hlog.FromRequest(r)
		if err := cache.Remove([]byte(key), logger); err != nil {
			logger.Error().Err(err).Msg("Unable to remove entry from cache")
			if errors.Is(err, database.ErrKeyNotFound) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	handler.HandleFunc("GET /cache/{key}", func(w http.ResponseWriter, r *http.Request) {
		logger := hlog.FromRequest(r)
		key := r.PathValue("key")
		entry := new(database.Entry[httpclient.CachedResponses])
		if err := cache.Get([]byte(key), entry); err != nil {
			logger.Error().Err(err).Msg("Unable to find entry in cache")
			if errors.Is(err, database.ErrKeyNotFound) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		if len(entry.Value) == 0 {
			logger.Error().Err(err).Msg("Entry in cache doesn't have any responses attached")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		resp := entry.Value[0]

		reader, err := cache.Open(resp.ContentHash, logger)
		if err != nil {
			logger.Error().Err(err).Msg("Entry in cache doesn't have the content available")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer func() {
			if err := reader.Close(); err != nil {
				logger.Error().Err(err).Msg("Unable to close file from cache after reading")
			}
		}()

		maps.Copy(w.Header(), resp.Headers)
		w.WriteHeader(resp.StatusCode)

		if _, err := io.Copy(w, reader); err != nil {
			logger.Error().Err(err).Msg("Something happened while sending data back to client")
		}
	})

	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		id, _ := hlog.IDFromRequest(r)

		stats, err := cache.GetStatistics(r.Context(), id.String())
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("unable to gather statistics")
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("Unable to gather statistics")); err != nil {
				hlog.FromRequest(r).Panic().Err(err).Msg("error returning an answer")
			}

			return
		}

		err = templates.ExecuteTemplate(
			w,
			"index.html.tmpl",
			indexData{version.Get(), stats, middlewareStats, renderedConfig},
		)
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
