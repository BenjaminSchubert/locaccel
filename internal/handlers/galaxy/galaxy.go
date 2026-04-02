package galaxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

var (
	ErrUnknownContentType = errors.New("unknown content type")
	ErrUnexpectedCDN      = errors.New("unexpected CDN requested")
)

type CollectionVersion struct {
	Artifact        json.RawMessage `json:"artifact"`
	Collection      json.RawMessage `json:"collection"`
	CreatedAt       string          `json:"created_at"`
	DownloadUrl     string          `json:"download_url"`
	Files           json.RawMessage `json:"files"`
	GiCommitSha     string          `json:"git_commit_sha"`
	GitUrl          string          `json:"git_url"`
	Href            string          `json:"href"`
	Manifest        json.RawMessage `json:"manifest"`
	Marks           json.RawMessage `json:"marks"`
	Metadata        json.RawMessage `json:"metadata"`
	Name            string          `json:"name"`
	Namespace       json.RawMessage `json:"namespace"`
	RequiresAnsible string          `json:"requires_ansible"`
	Signatures      json.RawMessage `json:"signatures"`
	UpdatedAt       string          `json:"updated_at"`
	Version         string          `json:"version"`
}

func RegisterHandler(
	galaxyServer string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}

	handler.HandleFunc(
		"GET /api/v3/collections/{namespace}/{name}/versions/{version}/",
		func(w http.ResponseWriter, r *http.Request) {
			handlers.Forward(
				w,
				r,
				galaxyServer+r.URL.RequestURI(),
				client,
				func(body []byte, resp *http.Response, handler *handlers.JSONHandler) error {
					switch resp.Header.Get("Content-Type") {
					case "application/json":
						return rewriteCollectionVersionV3(body, galaxyServer, handler)
					default:
						return fmt.Errorf(
							"%w: %s",
							ErrUnknownContentType,
							resp.Header.Get("Content-Type"),
						)
					}
				},
				caches,
			)
		},
	)
	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(w, r, galaxyServer+r.URL.RequestURI(), client, nil, caches)
	})
}

func rewriteCollectionVersionV3(
	body []byte,
	galaxyServer string,
	handler *handlers.JSONHandler,
) error {
	if _, err := handler.Buffer.Write(body); err != nil {
		return err
	}

	data := CollectionVersion{}
	if err := handler.Decoder.Decode(&data); err != nil {
		return err
	}

	if after, ok := strings.CutPrefix(data.DownloadUrl, galaxyServer); ok {
		data.DownloadUrl = after
	} else if data.DownloadUrl[0] != '/' {
		return fmt.Errorf("%w for %s", ErrUnexpectedCDN, data.DownloadUrl)
	}

	handler.Buffer.Reset()
	return handler.Encoder.Encode(data)
}
