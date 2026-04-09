package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/rs/zerolog/hlog"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/httpheaders"
)

var (
	jsonHandlerPool = sync.Pool{
		New: func() any {
			return NewJSONHandler()
		},
	}
	bytesPool = sync.Pool{
		New: func() any {
			buffer := make([]byte, 1024*32)
			return &buffer
		},
	}
	bufferPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
)

type JSONHandler struct {
	Buffer  *bytes.Buffer
	Decoder *json.Decoder
	Encoder *json.Encoder
}

func NewJSONHandler() *JSONHandler {
	buffer := new(bytes.Buffer)
	decoder := json.NewDecoder(buffer)
	decoder.DisallowUnknownFields()
	encoder := json.NewEncoder(buffer)
	return &JSONHandler{buffer, decoder, encoder}
}

func Forward(
	w http.ResponseWriter,
	r *http.Request,
	upstreamURL string,
	client *httpclient.Client,
	modify func(body []byte, resp *http.Response, jsonHandler *JSONHandler) error,
	recovery func(w http.ResponseWriter, err error) error,
	upstreamCache httpclient.UpstreamCache,
) {
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		hlog.FromRequest(r).Panic().Err(err).Msg("Error generating new upstream request")
	}
	maps.Copy(upstreamReq.Header, r.Header)

	resp, err := client.Do(upstreamReq, upstreamCache)
	if err != nil {
		if recovery != nil {
			if err2 := recovery(w, err); err2 != nil {
				hlog.FromRequest(r).
					Error().
					Err(err).
					Msg("An error happened when trying to recover from an error forwarding request to upstream")
			} else {
				return
			}
		}

		if uerr, ok := err.(*url.Error); ok {
			if uerr.Timeout() {
				w.WriteHeader(http.StatusGatewayTimeout)
			} else {
				w.WriteHeader(http.StatusBadGateway)
			}
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		hlog.FromRequest(r).
			Error().
			Err(err).
			Msg("Error forwarding request to upstream")

		return
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			hlog.FromRequest(r).
				Panic().
				Err(err).
				Msg("Error closing the body of the upstream request")
		}
	}()

	if resp.StatusCode == http.StatusOK && matchesOriginalQuery(r.Header, resp) {
		maps.Copy(w.Header(), resp.Header)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if modify != nil {
		buffer := bufferPool.Get().(*bytes.Buffer)
		defer func() {
			buffer.Reset()
			bufferPool.Put(buffer)
		}()

		if err := modifyBody(resp, r, modify, buffer); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			hlog.FromRequest(r).
				Error().
				Err(err).
				Msg("Error while preparing the body of the new response")
			return
		}
	}

	maps.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	buf := bytesPool.Get().(*[]byte)
	defer bytesPool.Put(buf)

	if n, err := io.CopyBuffer(
		struct{ io.Writer }{w},
		struct{ io.Reader }{resp.Body},
		*buf,
	); err != nil {
		if closer, ok := w.(io.Closer); ok {
			if err2 := closer.Close(); err2 != nil {
				hlog.FromRequest(r).
					Panic().
					Errs("errors", []error{err, err2}).
					Msg("Error closing the body of the upstream request after having a problem sending it the data")
			}
		}
		hlog.FromRequest(r).
			Error().
			Err(err).
			Int64("written", n).
			Msg("Error sending response to client")
	}
}

func modifyBody(
	resp *http.Response,
	r *http.Request,
	modify func(body []byte, resp *http.Response, jsonHandler *JSONHandler) error,
	buffer *bytes.Buffer,
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

	if isGzipped {
		if cErr := body.Close(); cErr != nil {
			hlog.FromRequest(r).
				Error().
				Err(cErr).
				Msg("An unexpected error happend trying to close the gzip reader")
		}
	}

	if cErr := resp.Body.Close(); cErr != nil {
		hlog.FromRequest(r).
			Error().
			Err(cErr).
			Msg("An unexpected error happend trying to close the upstream request body")
	}

	if err != nil {
		return err
	}

	jsonHandler := jsonHandlerPool.Get().(*JSONHandler)
	defer func() {
		jsonHandler.Buffer.Reset()
		jsonHandlerPool.Put(jsonHandler)
	}()

	if err := modify(content, resp, jsonHandler); err != nil {
		return err
	}
	if isGzipped {
		writer := gzip.NewWriter(buffer)
		_, err := writer.Write(jsonHandler.Buffer.Bytes())
		if err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
	} else {
		buffer.Write(jsonHandler.Buffer.Bytes())
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
