package middleware

import (
	"log"
	"net/http"
)

type Middleware func(http.Handler) http.Handler

var Logging Middleware = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request: %s %s", r.Method, r.URL.Path)

		next.ServeHTTP(w, r)
	})
}
