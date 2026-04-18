package middleware

import (
	"context"
	"net/http"
)

func Platform(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platform := r.Header.Get("X-Platform")
		if platform == "" {
			platform = "web"
		}

		ctx := context.WithValue(r.Context(), ContextPlatformKey, platform)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
