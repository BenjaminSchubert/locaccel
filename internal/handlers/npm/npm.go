package npm

import (
	"bytes"
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

type Dist struct {
	Attestations json.RawMessage `json:"attestations,omitempty"`
	FileCount    int             `json:"fileCount"`
	Integrity    string          `json:"integrity"`
	NpmSignature string          `json:"npm-signature,omitempty"`
	Shasum       string          `json:"shasum"`
	Signatures   json.RawMessage `json:"signatures,omitempty"`
	Tarball      string          `json:"tarball"`
	UnpackedSize int             `json:"unpackedSize"`
}

type Version struct {
	Dependencies     json.RawMessage `json:"dependencies,omitempty"`
	Deprecated       string          `json:"deprecated,omitempty"`
	DevDependencies  json.RawMessage `json:"devDependencies,omitempty"`
	Dist             Dist            `json:"dist"`
	Engines          json.RawMessage `json:"engines"`
	PeerDependencies json.RawMessage `json:"peerDependencies,omitempty"`
	Version          string          `json:"version"`
	Name             string          `json:"name"`
}

type NpmProject struct {
	DistTag  json.RawMessage     `json:"dist-tags"`
	Modified string              `json:"modified"`
	Name     string              `json:"name"`
	Versions map[string]*Version `json:"versions"`
}

func RegisterHandler(
	upstream, scheme string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	// Upstream must not end with /
	if upstream[len(upstream)-1] == '/' {
		upstream = upstream[:len(upstream)-1]
	}

	upstreamCacheUrls := make([]string, 0, len(upstreamCaches))
	for _, upstream := range upstreamCaches {
		upstreamCacheUrls = append(upstreamCacheUrls, upstream.String())
	}
	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}

	handler.HandleFunc("GET /{pkg}/-/{path}", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(w, r, upstream+r.URL.RequestURI(), client, nil, caches)
	})
	handler.HandleFunc(
		"GET /{namespace}/{pkg}/-/{path}",
		func(w http.ResponseWriter, r *http.Request) {
			handlers.Forward(w, r, upstream+r.URL.RequestURI(), client, nil, caches)
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
					return rewriteJson(body, r, upstream, scheme, upstreamCacheUrls)
				default:
					return nil, fmt.Errorf(
						"%w: %s",
						ErrUnknownContentType,
						resp.Header.Get("Content-Type"),
					)
				}
			},
			caches,
		)
	})
}

func rewriteJson(
	body []byte,
	r *http.Request,
	upstream, scheme string,
	upstreamCaches []string,
) ([]byte, error) {
	buf := bytes.NewBuffer(body)
	decoder := json.NewDecoder(buf)
	decoder.DisallowUnknownFields()
	data := NpmProject{}
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	remote := ""

	for _, version := range data.Versions {
		if remote == "" {
			if strings.HasPrefix(version.Dist.Tarball, upstream) {
				remote = upstream
			} else {
				for _, upstream := range upstreamCaches {
					if strings.HasPrefix(version.Dist.Tarball, upstream) {
						remote = upstream
						break
					}
				}
			}

			if remote == "" {
				return nil, fmt.Errorf("%w for %s", ErrUnexpectedCDN, version.Dist.Tarball)
			}
		}

		version.Dist.Tarball = scheme + "://" + r.Host + strings.TrimPrefix(
			version.Dist.Tarball,
			remote,
		)
	}

	buf.Reset()
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
