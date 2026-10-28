package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxRequestID
)

func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxUserID).(int64)
	return id
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func AccessLog(log *slog.Logger, trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			holder := &userHolder{}
			r = r.WithContext(context.WithValue(r.Context(), userHolderKey{}, holder))
			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			attrs := []any{
				"request_id", r.Context().Value(ctxRequestID),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_ip", clientIP(r, trustProxy),
			}
			if holder.id != 0 {
				attrs = append(attrs, "user_id", holder.id)
			}
			log.Log(r.Context(), level, "http request", attrs...)
		})
	}
}

type userHolderKey struct{}

type userHolder struct{ id int64 }

func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.ErrorContext(r.Context(), "panic", "value", v, "stack", string(debug.Stack()))
					WriteErrorCode(w, http.StatusInternalServerError, "internal_error", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func LimitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

type TokenParser interface {
	ParseAccessToken(token string) (int64, error)
}

func Authenticate(tp TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				WriteErrorCode(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			userID, err := tp.ParseAccessToken(token)
			if err != nil {
				WriteErrorCode(w, http.StatusUnauthorized, "unauthorized", "access token is invalid or expired")
				return
			}
			if h, ok := r.Context().Value(userHolderKey{}).(*userHolder); ok {
				h.id = userID
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID, userID)))
		})
	}
}

type RateLimitOptions struct {
	RPS               float64
	Burst             int
	TrustProxyHeaders bool
	ClientTTL         time.Duration
	CleanupInterval   time.Duration
}

type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*client
	opts    RateLimitOptions
}

type client struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(ctx context.Context, opts RateLimitOptions) *RateLimiter {
	rl := &RateLimiter{clients: map[string]*client{}, opts: opts}
	go rl.cleanup(ctx)
	return rl
}

func (rl *RateLimiter) cleanup(ctx context.Context) {
	t := time.NewTicker(rl.opts.CleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.mu.Lock()
			for ip, c := range rl.clients {
				if time.Since(c.lastSeen) > rl.opts.ClientTTL {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	c, ok := rl.clients[ip]
	if !ok {
		c = &client{lim: rate.NewLimiter(rate.Limit(rl.opts.RPS), rl.opts.Burst)}
		rl.clients[ip] = c
	}
	c.lastSeen = time.Now()
	return c.lim.Allow()
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r, rl.opts.TrustProxyHeaders)) {
			w.Header().Set("Retry-After", "1")
			WriteErrorCode(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
