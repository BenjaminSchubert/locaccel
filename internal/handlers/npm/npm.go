package npm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/benjaminschubert/locaccel/internal/handlers"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

var ErrUnknownContentType = errors.New("unknown content type")

func RegisterHandler(upstream, scheme string, handler *http.ServeMux, client *httpclient.Client) {
	// Upstream must not end with /
	if upstream[len(upstream)-1] == '/' {
		upstream = upstream[:len(upstream)-1]
	}

	handler.HandleFunc("GET /{pkg}/-/{path}", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(w, r, upstream+r.URL.RequestURI(), client, nil)
	})
	handler.HandleFunc(
		"GET /{namespace}/{pkg}/-/{path}",
		func(w http.ResponseWriter, r *http.Request) {
			handlers.Forward(w, r, upstream+r.URL.RequestURI(), client, nil)
		},
	)

	handler.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			upstream+r.URL.RequestURI(),
			client,
			func(body []byte, resp *http.Response) ([]byte, error) {
				switch resp.Header.Get("Content-Type") {
				case "application/vnd.npm.install-v1+json":
					return rewriteJson(body, r, upstream, scheme)
				default:
					return nil, fmt.Errorf(
						"%w: %s",
						ErrUnknownContentType,
						resp.Header.Get("Content-Type"),
					)
				}
			},
		)
	})
}

func rewriteJson(body []byte, r *http.Request, upstream, scheme string) ([]byte, error) {
	buf := bytes.NewBuffer(body)
	decoder := json.NewDecoder(buf)
	data := make(map[string]any)
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	for _, info := range data["versions"].(map[string]any) {
		dist := info.(map[string]any)["dist"].(map[string]any)
		dist["tarball"] = scheme + "://" + r.Host + strings.TrimPrefix(
			dist["tarball"].(string),
			upstream,
		)
	}

	buf.Reset()
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
