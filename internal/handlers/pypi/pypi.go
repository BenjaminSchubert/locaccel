package pypi

import (
	"bytes"
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

	// Index files
	handler.HandleFunc("GET /simple/", func(w http.ResponseWriter, r *http.Request) {
		handlers.Forward(
			w,
			r,
			upstream+r.URL.RequestURI(),
			client,
			func(body []byte, resp *http.Response) ([]byte, error) {
				switch resp.Header.Get("Content-Type") {
				case "application/vnd.pypi.simple.v1+json":
					return rewriteJsonV1(body, expectedCDN, encodedCDN)
				default:
					return nil, fmt.Errorf(
						"%w: %s",
						ErrUnknownContentType,
						resp.Header.Get("Content-Type"),
					)
				}
			},
			upstreamCaches,
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
				upstreamCachesWithCDN,
			)
		},
	)
}

func rewriteJsonV1(body []byte, expectedCDN, encodedCDN string) ([]byte, error) {
	buf := bytes.NewBuffer(body)
	decoder := json.NewDecoder(buf)
	data := make(map[string]any)
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	expectedPrefix := ""

	for _, fileInfo := range data["files"].([]any) {
		file := fileInfo.(map[string]any)
		originalUrl := file["url"].(string)

		if expectedPrefix == "" {
			switch {
			case strings.HasPrefix(originalUrl, expectedCDN):
				expectedPrefix = expectedCDN
			case strings.HasPrefix(originalUrl, encodedCDN):
				expectedPrefix = encodedCDN
				encodedCDN = ""
			default:
				return nil, fmt.Errorf("%w for %s", ErrUnexpectedCDN, originalUrl)
			}
		}

		if !strings.HasPrefix(originalUrl, expectedPrefix) {
			return nil, fmt.Errorf("%w for %s", ErrUnexpectedCDN, originalUrl)
		}

		uri, err := url.Parse(originalUrl)
		if err != nil {
			return nil, err
		}

		// Rewrite the url to point to here
		uri.Host = ""
		uri.Scheme = ""
		uri.Path = encodedCDN + uri.Path

		file["url"] = uri.String()
	}

	buf.Reset()
	encoder := json.NewEncoder(buf)
	err := encoder.Encode(data)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
