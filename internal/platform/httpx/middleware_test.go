package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestLimiter(t *testing.T, opts RateLimitOptions) (*RateLimiter, http.Handler) {
	t.Helper()
	if opts.ClientTTL == 0 {
		opts.ClientTTL = time.Hour
	}
	if opts.CleanupInterval == 0 {
		opts.CleanupInterval = time.Hour
	}
	rl := NewRateLimiter(t.Context(), opts)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return rl, rl.Middleware(ok)
}

func call(h http.Handler, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit_BurstThenTooManyRequests(t *testing.T) {
	_, h := newTestLimiter(t, RateLimitOptions{RPS: 0.001, Burst: 3})

	for i := range 3 {
		if rec := call(h, "10.0.0.1:1000", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d within burst: status %d", i+1, rec.Code)
		}
	}
	rec := call(h, "10.0.0.1:1001", nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request over burst: status %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != "rate_limited" {
		t.Errorf("body = %s, want error code rate_limited", rec.Body)
	}
}

func TestRateLimit_BucketPerClientIP(t *testing.T) {
	_, h := newTestLimiter(t, RateLimitOptions{RPS: 0.001, Burst: 1})

	if rec := call(h, "10.0.0.1:1000", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("first client: status %d", rec.Code)
	}
	if rec := call(h, "10.0.0.1:2000", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("same IP from another port must share the bucket: status %d", rec.Code)
	}
	if rec := call(h, "10.0.0.2:1000", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("another IP must have its own bucket: status %d", rec.Code)
	}
}

func TestRateLimit_RefillsOverTime(t *testing.T) {
	_, h := newTestLimiter(t, RateLimitOptions{RPS: 50, Burst: 1})

	call(h, "10.0.0.1:1000", nil)
	if rec := call(h, "10.0.0.1:1000", nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second immediate request: status %d, want 429", rec.Code)
	}
	time.Sleep(40 * time.Millisecond)
	if rec := call(h, "10.0.0.1:1000", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("after refill: status %d", rec.Code)
	}
}

func TestRateLimit_ProxyHeadersOnlyWhenTrusted(t *testing.T) {
	spoofed := map[string]string{"X-Real-IP": "203.0.113.7"}

	_, h := newTestLimiter(t, RateLimitOptions{RPS: 0.001, Burst: 1})
	call(h, "10.0.0.1:1000", spoofed)
	if rec := call(h, "10.0.0.1:1000", map[string]string{"X-Real-IP": "203.0.113.8"}); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("untrusted proxy headers must be ignored: status %d", rec.Code)
	}

	_, h = newTestLimiter(t, RateLimitOptions{RPS: 0.001, Burst: 1, TrustProxyHeaders: true})
	call(h, "10.0.0.1:1000", spoofed)
	if rec := call(h, "10.0.0.1:1000", map[string]string{"X-Real-IP": "203.0.113.8"}); rec.Code != http.StatusNoContent {
		t.Fatalf("trusted X-Real-IP must define the client: status %d", rec.Code)
	}
}

func TestRateLimit_CleanupForgetsIdleClients(t *testing.T) {
	rl, h := newTestLimiter(t, RateLimitOptions{
		RPS: 0.001, Burst: 1, ClientTTL: 20 * time.Millisecond, CleanupInterval: 5 * time.Millisecond,
	})
	call(h, "10.0.0.1:1000", nil)

	deadline := time.Now().Add(2 * time.Second)
	for {
		rl.mu.Lock()
		n := len(rl.clients)
		rl.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle client still tracked: %d entries", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rec := call(h, "10.0.0.1:1000", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("forgotten client must start with a full bucket: status %d", rec.Code)
	}
}
