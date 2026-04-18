package middleware

import (
	"net/http"
	"slices"
)

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, role, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, `{"code":1002,"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if !slices.Contains(roles, role) {
				http.Error(w, `{"code":1003,"message":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
