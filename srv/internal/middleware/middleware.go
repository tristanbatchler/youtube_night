package middleware

import (
	"log"
	"net/http"
)

type Middleware func(http.Handler) http.Handler

var Logging Middleware = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff := r.Header.Get("X-Forwarded-For")
		log.Printf("Request: %s %s from %s", r.Method, r.URL.Path, xff)

		next.ServeHTTP(w, r)
	})
}

// For injecting the content type header
var ContentType Middleware = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

// ChainMiddleware allows chaining multiple middlewares
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}
