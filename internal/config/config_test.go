package config

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.WriteTimeout != 30*time.Second || cfg.HTTP.MaxBodyBytes != 1<<20 {
		t.Errorf("unexpected HTTP defaults: %+v", cfg.HTTP)
	}
	if cfg.MySQL.MaxOpenConns != 25 || cfg.MySQL.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("unexpected MySQL pool defaults: %+v", cfg.MySQL)
	}
	if cfg.Cache.BreakerFailures != 5 || cfg.Cache.BreakerOpenTimeout != 10*time.Second {
		t.Errorf("unexpected cache defaults: %+v", cfg.Cache)
	}
	if cfg.Startup.RetryAttempts != 30 || cfg.RateLimit.ClientTTL != 3*time.Minute {
		t.Errorf("unexpected startup/rate limit defaults: %+v %+v", cfg.Startup, cfg.RateLimit)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	t.Setenv("HTTP_WRITE_TIMEOUT", "45s")
	t.Setenv("MYSQL_MAX_OPEN_CONNS", "50")
	t.Setenv("CACHE_BREAKER_FAILURES", "3")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.WriteTimeout != 45*time.Second || cfg.MySQL.MaxOpenConns != 50 || cfg.Cache.BreakerFailures != 3 {
		t.Errorf("overrides not applied: %+v %+v %+v", cfg.HTTP, cfg.MySQL, cfg.Cache)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	t.Setenv("HTTP_READ_TIMEOUT", "soon")
	t.Setenv("CACHE_BREAKER_FAILURES", "0")
	t.Setenv("MYSQL_CONN_MAX_LIFETIME", "-1m")
	t.Setenv("LOG_LEVEL", "verbose")
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"HTTP_READ_TIMEOUT", "CACHE_BREAKER_FAILURES must be positive", "MYSQL_CONN_MAX_LIFETIME must be positive", "LOG_LEVEL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
	if n := strings.Count(err.Error(), "HTTP_READ_TIMEOUT"); n != 1 {
		t.Errorf("malformed HTTP_READ_TIMEOUT reported %d times, want 1: %q", n, err)
	}
}
