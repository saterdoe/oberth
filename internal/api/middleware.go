package api

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// wrappedWriter captures the status code written by downstream handlers.
type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

var _ http.Hijacker = (*wrappedWriter)(nil)
var _ http.Flusher = (*wrappedWriter)(nil)

func (w *wrappedWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Unwrap lets net/http.ResponseController reach optional interfaces provided
// by the underlying server writer.
func (w *wrappedWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Hijack preserves WebSocket upgrades through the request logger.
func (w *wrappedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Flush preserves streaming responses through the request logger.
func (w *wrappedWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// GetRequestID extracts the request ID from the context.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID injects or propagates an X-Request-ID header and stores it in the
// request context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logger logs each HTTP request with method, path, response status, and
// duration using slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		slog.Info("request",
			"request_id", GetRequestID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", time.Since(start).String(),
		)
	})
}

// Recoverer catches panics in downstream handlers, logs them, and returns a
// 500 Internal Server Error JSON response.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec)
				respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders applies browser hardening to every daemon response. The API
// never renders executable content, so these policies do not alter API behavior.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func LocalAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		provided := ""
		const prefix = "Bearer "
		if header := r.Header.Get("Authorization"); len(header) > len(prefix) && header[:len(prefix)] == prefix {
			provided = header[len(prefix):]
		}
		if provided == "" && r.URL.Path == "/ws/v1/events" {
			provided = websocketProtocolToken(r.Header.Get("Sec-WebSocket-Protocol"))
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			respondError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "valid local daemon token required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const websocketTokenProtocolPrefix = "oberth-token."

func websocketProtocolToken(protocols string) string {
	protocol := websocketAuthProtocol(protocols)
	encoded, found := strings.CutPrefix(protocol, websocketTokenProtocolPrefix)
	if !found {
		return ""
	}
	token, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return string(token)
}

func websocketAuthProtocol(protocols string) string {
	for protocol := range strings.SplitSeq(protocols, ",") {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, websocketTokenProtocolPrefix) {
			return protocol
		}
	}
	return ""
}

func LocalHostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsed
		}
		switch host {
		case "localhost", "127.0.0.1", "::1":
			next.ServeHTTP(w, r)
		default:
			respondError(w, http.StatusForbidden, "HOST_DENIED", "daemon only accepts local hostnames", nil)
		}
	})
}

var allowedLocalOrigins = map[string]struct{}{
	"http://127.0.0.1:5173":   {},
	"http://127.0.0.1:5174":   {},
	"http://localhost:5173":   {},
	"http://localhost:5174":   {},
	"http://wails.localhost":  {},
	"https://wails.localhost": {},
	"wails://wails.localhost": {},
}

// CORS only admits the exact local UI origins. Invalid preflights are rejected
// instead of falling through to an API handler.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			_, allowed := allowedLocalOrigins[origin]
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || !allowed {
				respondError(w, http.StatusForbidden, "ORIGIN_DENIED", "origin is not allowed", nil)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
