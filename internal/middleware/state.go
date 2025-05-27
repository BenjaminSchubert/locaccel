package middleware

import (
	"context"
	"net/http"
)

type ctxStateKeyStruct struct{}

var ctxStateKey = ctxStateKeyStruct{}

type RequestState struct {
	cache string
}

func initializeState(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxStateKey, &RequestState{})
}

func SetCacheState(r *http.Request, value string) {
	r.Context().Value(ctxStateKey).(*RequestState).cache = value
}

func GetCacheState(ctx context.Context) string {
	val := ctx.Value(ctxStateKey).(*RequestState).cache
	if val == "" {
		return "N/A"
	}
	return val
}

func StateHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(initializeState(r.Context()))
		next.ServeHTTP(w, r)
	})
}
