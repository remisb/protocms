package auth

import (
	"context"
	"net/http"
)

type contextKey string

const datasetKey contextKey = "dataset"

// WithDataset is middleware that resolves the dataset bound to the request's
// bearer token and stores it in the request context. It must run after the
// Authenticator (the token is already known to be valid); an unresolvable
// token yields a 401.
//
// This exists because the external middleware.Claims type cannot carry a
// dataset field, so ProtoCMS injects the dataset under its own context key.
func (c Config) WithDataset(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ds, ok := c.ResolveDataset(token)
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), datasetKey, ds))
		next.ServeHTTP(w, r)
	})
}

// DatasetFromContext returns the dataset resolved by WithDataset, or
// ("", false) if none is present.
func DatasetFromContext(ctx context.Context) (string, bool) {
	ds, ok := ctx.Value(datasetKey).(string)
	return ds, ok && ds != ""
}

// bearerToken extracts a bearer token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) < len(prefix) || h[:len(prefix)] != prefix {
		return "", false
	}
	return h[len(prefix):], true
}
