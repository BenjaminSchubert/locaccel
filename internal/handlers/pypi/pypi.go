package pypi

import (
	"encoding/base64"
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

type File struct {
	CoreMetadata         json.RawMessage `json:"core-metadata"`
	DataDistInfoMetadata json.RawMessage `json:"data-dist-info-metadata"`
	Filename             string          `json:"filename"`
	Hashes               json.RawMessage `json:"hashes"`
	Provenance           string          `json:"provenance"`
	RequiresPython       string          `json:"requires-python"`
	Size                 int             `json:"size"`
	UploadTime           string          `json:"upload-time"`
	Yanked               json.RawMessage `json:"yanked"`
	Url                  string          `json:"url"`
}
type PypiProject struct {
	Files              []File          `json:"files"`
	AlternateLocations json.RawMessage `json:"alternate-locations"`
	Meta               json.RawMessage `json:"meta"`
	Name               string          `json:"name"`
	ProjectStatus      json.RawMessage `json:"project-status"`
	Versions           json.RawMessage `json:"versions"`
}

func RegisterHandler(
	upstream, expectedCDN string,
	handler *http.ServeMux,
	client *httpclient.Client,
	upstreamCaches []*url.URL,
) {
	// Upstream must not end with /
	if upstream[len(upstream)-1] == '/' {
		upstream = upstream[:len(upstream)-1]
	}
	// CDN must end with /
	if expectedCDN[len(expectedCDN)-1] != '/' {
		expectedCDN += "/"
	}
	encodedCDN := "/cdn/" + base64.StdEncoding.EncodeToString([]byte(expectedCDN))

	upstreamCachesWithCDN := make([]*url.URL, 0, len(upstreamCaches))
	for _, upstream := range upstreamCaches {
		up := new(url.URL)
		*up = *upstream
		up.Path += encodedCDN
		upstreamCachesWithCDN = append(upstreamCachesWithCDN, up)
	}

	caches := httpclient.UpstreamCache{Uris: upstreamCaches, Proxy: false}
	cachesWithCDN := httpclient.UpstreamCache{Uris: upstreamCachesWithCDN, Proxy: false}

	// Index files
	handler.HandleFunc("GET /simple/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			upstream+r.URL.RequestURI(),
			client,
			func(body []byte, resp *http.Response, jsonHandler *handlers.JSONHandler) error {
				switch resp.Header.Get("Content-Type") {
				case "application/vnd.pypi.simple.v1+json":
					return rewriteJsonV1(body, expectedCDN, encodedCDN, jsonHandler)
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
	})

	handler.HandleFunc(
		"GET "+encodedCDN+"/{path...}",
		func(w http.ResponseWriter, r *http.Request) {
			handlers.Forward(
				w,
				r,
				expectedCDN+r.PathValue("path"),
				client,
				nil,
				cachesWithCDN,
			)
		},
	)
}

func rewriteJsonV1(
	body []byte,
	expectedCDN, encodedCDN string,
	handler *handlers.JSONHandler,
) error {
	if _, err := handler.Buffer.Write(body); err != nil {
		return err
	}

	data := PypiProject{}
	if err := handler.Decoder.Decode(&data); err != nil {
		return err
	}

	expectedPrefix := ""

	for i := range data.Files {
		originalUrl := data.Files[i].Url

		if expectedPrefix == "" {
			switch {
			case strings.HasPrefix(originalUrl, expectedCDN):
				expectedPrefix = expectedCDN
			case strings.HasPrefix(originalUrl, encodedCDN):
				expectedPrefix = encodedCDN
				encodedCDN = ""
			default:
				return fmt.Errorf("%w for %s", ErrUnexpectedCDN, originalUrl)
			}
		}

		if !strings.HasPrefix(originalUrl, expectedPrefix) {
			return fmt.Errorf("%w for %s", ErrUnexpectedCDN, originalUrl)
		}

		uri, err := url.Parse(originalUrl)
		if err != nil {
			return err
		}

		// Rewrite the url to point to here
		uri.Host = ""
		uri.Scheme = ""
		uri.Path = encodedCDN + uri.Path

		data.Files[i].Url = uri.String()
	}

	handler.Buffer.Reset()
	return handler.Encoder.Encode(data)
}
