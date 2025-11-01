package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strconv"
	"sync"

	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/httpheaders"
)

var JSONHandlerPool = sync.Pool{
	New: func() any {
		buffer := new(bytes.Buffer)
		decoder := json.NewDecoder(buffer)
		decoder.DisallowUnknownFields()
		encoder := json.NewEncoder(buffer)
		return &JSONHandler{buffer, decoder, encoder}
	},
}

type JSONHandler struct {
	Buffer  *bytes.Buffer
	Decoder *json.Decoder
	Encoder *json.Encoder
}

func Forward(
	w http.ResponseWriter,
	r *http.Request,
	upstreamURL string,
	client *httpclient.Client,
	modify func(body []byte, resp *http.Response) ([]byte, error),
	upstreamCache httpclient.UpstreamCache,
) {
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		hlog.FromRequest(r).Panic().Err(err).Msg("Error generating new upstream request")
	}
	maps.Copy(upstreamReq.Header, r.Header)

	resp, err := client.Do(upstreamReq, upstreamCache) //nolint:bodyclose
	if err != nil {
		hlog.FromRequest(r).
			Panic().
			Err(err).
			Msg("Error forwarding request to upstream")
	}
	body := resp.Body

	defer func() {
		if body.Close() != nil {
			hlog.FromRequest(r).
				Panic().
				Err(err).
				Msg("Error closing the body of the upstream request")
		}
	}()

	maps.Copy(w.Header(), resp.Header)

	if resp.StatusCode == http.StatusOK && matchesOriginalQuery(r.Header, resp) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if modify != nil {
		if err := modifyBody(resp, modify); err != nil {
			hlog.FromRequest(r).
				Panic().
				Err(err).
				Msg("Error while preparing the body of the new response")
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		hlog.FromRequest(r).Panic().Err(err).Msg("Error sending response to client")
	}
}

func modifyBody(
	resp *http.Response,
	modify func(body []byte, resp *http.Response) ([]byte, error),
) (err error) {
	isGzipped := resp.Header.Get("Content-Encoding") == "gzip"
	body := resp.Body

	if isGzipped {
		body, err = gzip.NewReader(body)
		if err != nil {
			return err
		}
	}

	content, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	newContent, err := modify(content, resp)
	if err != nil {
		return err
	}
	var buffer *bytes.Buffer

	if isGzipped {
		buffer = bytes.NewBuffer(nil)
		writer := gzip.NewWriter(buffer)
		_, err := writer.Write(newContent)
		if err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
	} else {
		buffer = bytes.NewBuffer(newContent)
	}

	if resp.Header["Content-Length"] != nil {
		resp.Header["Content-Length"] = []string{strconv.Itoa(buffer.Len())}
	}

	resp.Body = io.NopCloser(buffer)
	return nil
}

func matchesOriginalQuery(headers http.Header, resp *http.Response) bool {
	etag := resp.Header.Get("Etag")
	if etag != "" {
		for _, match := range headers["If-None-Match"] {
			if httpheaders.EtagsMatch(etag, match) {
				return true
			}
		}
	}

	lastModified := resp.Header.Get("Last-Modified")
	return lastModified != "" && lastModified == headers.Get("If-Modified-Since")
}
