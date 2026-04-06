package middleware

import (
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Logger logs each request with method, path, status and duration
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		dur := time.Since(start)
		// Skip logging static/poll requests to reduce noise
		if r.URL.Path != "/" && !strings.HasSuffix(r.URL.Path, "/status") {
			log.Printf("%s %s %d %dms", r.Method, r.URL.Path, rw.status, dur.Milliseconds())
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Gzip compresses responses when client supports it
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.writer.Write(b)
}

// RateLimit limits requests per IP to prevent abuse
func RateLimit(rps int) func(http.Handler) http.Handler {
	type entry struct {
		count    int
		resetAt  time.Time
	}
	var mu sync.Mutex
	clients := make(map[string]*entry)

	// Cleanup old entries every minute
	go func() {
		for range time.Tick(time.Minute) {
			mu.Lock()
			now := time.Now()
			for ip, e := range clients {
				if now.After(e.resetAt) {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only rate-limit API endpoints
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			ip := r.RemoteAddr
			if idx := strings.LastIndex(ip, ":"); idx != -1 {
				ip = ip[:idx]
			}
			mu.Lock()
			e, ok := clients[ip]
			if !ok || time.Now().After(e.resetAt) {
				clients[ip] = &entry{count: 1, resetAt: time.Now().Add(time.Second)}
				mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}
			e.count++
			over := e.count > rps
			mu.Unlock()
			if over {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies multiple middlewares in order
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
