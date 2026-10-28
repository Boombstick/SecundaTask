package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	LogLevel       string
	MigrateOnStart bool

	HTTP      HTTP
	Startup   Startup
	MySQL     MySQL
	Redis     Redis
	Auth      Auth
	Cache     Cache
	RateLimit RateLimit
}

type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
	TrustProxyHeaders bool
}

type Startup struct {
	RetryAttempts int
	RetryInterval time.Duration
}

type MySQL struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC&time_zone=%%27%%2B00%%3A00%%27&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		m.User, m.Password, m.Host, m.Port, m.Database)
}

func (m MySQL) MigrateDSN() string {
	return m.DSN() + "&multiStatements=true"
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

type Auth struct {
	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type Cache struct {
	TTL                     time.Duration
	InvalidateTimeout       time.Duration
	InvalidateRetryInterval time.Duration
	BreakerFailures         int
	BreakerOpenTimeout      time.Duration
	BreakerHalfOpenRequests int
}

type RateLimit struct {
	RPS             float64
	Burst           int
	ClientTTL       time.Duration
	CleanupInterval time.Duration
}

func Load() (Config, error) {
	var errs []error
	cfg := Config{
		LogLevel:       strings.ToLower(str("LOG_LEVEL", "info")),
		MigrateOnStart: boolean("MIGRATE_ON_START", true, &errs),
		HTTP: HTTP{
			Addr:              str("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: dur("HTTP_READ_HEADER_TIMEOUT", 5*time.Second, &errs),
			ReadTimeout:       dur("HTTP_READ_TIMEOUT", 15*time.Second, &errs),
			WriteTimeout:      dur("HTTP_WRITE_TIMEOUT", 30*time.Second, &errs),
			IdleTimeout:       dur("HTTP_IDLE_TIMEOUT", 60*time.Second, &errs),
			ShutdownTimeout:   dur("SHUTDOWN_TIMEOUT", 15*time.Second, &errs),
			MaxBodyBytes:      int64(integer("HTTP_MAX_BODY_BYTES", 1<<20, &errs)),
			TrustProxyHeaders: boolean("TRUST_PROXY_HEADERS", false, &errs),
		},
		Startup: Startup{
			RetryAttempts: integer("STARTUP_RETRY_ATTEMPTS", 30, &errs),
			RetryInterval: dur("STARTUP_RETRY_INTERVAL", 2*time.Second, &errs),
		},
		MySQL: MySQL{
			Host:            str("MYSQL_HOST", "localhost"),
			Port:            integer("MYSQL_PORT", 3306, &errs),
			User:            str("MYSQL_USER", "tasks"),
			Password:        str("MYSQL_PASSWORD", ""),
			Database:        str("MYSQL_DATABASE", "tasks"),
			MaxOpenConns:    integer("MYSQL_MAX_OPEN_CONNS", 25, &errs),
			MaxIdleConns:    integer("MYSQL_MAX_IDLE_CONNS", 25, &errs),
			ConnMaxLifetime: dur("MYSQL_CONN_MAX_LIFETIME", 5*time.Minute, &errs),
		},
		Redis: Redis{
			Addr:     str("REDIS_ADDR", "localhost:6379"),
			Password: str("REDIS_PASSWORD", ""),
			DB:       integer("REDIS_DB", 0, &errs),
		},
		Auth: Auth{
			JWTSecret:  str("JWT_SECRET", ""),
			AccessTTL:  dur("JWT_ACCESS_TTL", 15*time.Minute, &errs),
			RefreshTTL: dur("JWT_REFRESH_TTL", 30*24*time.Hour, &errs),
		},
		Cache: Cache{
			TTL:                     dur("CACHE_TASKS_TTL", 5*time.Minute, &errs),
			InvalidateTimeout:       dur("CACHE_INVALIDATE_TIMEOUT", 2*time.Second, &errs),
			InvalidateRetryInterval: dur("CACHE_INVALIDATE_RETRY_INTERVAL", time.Second, &errs),
			BreakerFailures:         integer("CACHE_BREAKER_FAILURES", 5, &errs),
			BreakerOpenTimeout:      dur("CACHE_BREAKER_OPEN_TIMEOUT", 10*time.Second, &errs),
			BreakerHalfOpenRequests: integer("CACHE_BREAKER_HALF_OPEN_REQUESTS", 1, &errs),
		},
		RateLimit: RateLimit{
			RPS:             float("RATE_LIMIT_RPS", 20, &errs),
			Burst:           integer("RATE_LIMIT_BURST", 40, &errs),
			ClientTTL:       dur("RATE_LIMIT_CLIENT_TTL", 3*time.Minute, &errs),
			CleanupInterval: dur("RATE_LIMIT_CLEANUP_INTERVAL", time.Minute, &errs),
		},
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, cfg.LogLevel) {
		errs = append(errs, errors.New("LOG_LEVEL must be one of: debug, info, warn, error"))
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be set and at least 32 characters long"))
	}
	errs = append(errs, positive(map[string]float64{
		"HTTP_READ_HEADER_TIMEOUT":         cfg.HTTP.ReadHeaderTimeout.Seconds(),
		"HTTP_READ_TIMEOUT":                cfg.HTTP.ReadTimeout.Seconds(),
		"HTTP_WRITE_TIMEOUT":               cfg.HTTP.WriteTimeout.Seconds(),
		"HTTP_IDLE_TIMEOUT":                cfg.HTTP.IdleTimeout.Seconds(),
		"SHUTDOWN_TIMEOUT":                 cfg.HTTP.ShutdownTimeout.Seconds(),
		"HTTP_MAX_BODY_BYTES":              float64(cfg.HTTP.MaxBodyBytes),
		"STARTUP_RETRY_ATTEMPTS":           float64(cfg.Startup.RetryAttempts),
		"STARTUP_RETRY_INTERVAL":           cfg.Startup.RetryInterval.Seconds(),
		"MYSQL_MAX_OPEN_CONNS":             float64(cfg.MySQL.MaxOpenConns),
		"MYSQL_MAX_IDLE_CONNS":             float64(cfg.MySQL.MaxIdleConns),
		"MYSQL_CONN_MAX_LIFETIME":          cfg.MySQL.ConnMaxLifetime.Seconds(),
		"JWT_ACCESS_TTL":                   cfg.Auth.AccessTTL.Seconds(),
		"JWT_REFRESH_TTL":                  cfg.Auth.RefreshTTL.Seconds(),
		"CACHE_TASKS_TTL":                  cfg.Cache.TTL.Seconds(),
		"CACHE_INVALIDATE_TIMEOUT":         cfg.Cache.InvalidateTimeout.Seconds(),
		"CACHE_INVALIDATE_RETRY_INTERVAL":  cfg.Cache.InvalidateRetryInterval.Seconds(),
		"CACHE_BREAKER_FAILURES":           float64(cfg.Cache.BreakerFailures),
		"CACHE_BREAKER_OPEN_TIMEOUT":       cfg.Cache.BreakerOpenTimeout.Seconds(),
		"CACHE_BREAKER_HALF_OPEN_REQUESTS": float64(cfg.Cache.BreakerHalfOpenRequests),
		"RATE_LIMIT_RPS":                   cfg.RateLimit.RPS,
		"RATE_LIMIT_BURST":                 float64(cfg.RateLimit.Burst),
		"RATE_LIMIT_CLIENT_TTL":            cfg.RateLimit.ClientTTL.Seconds(),
		"RATE_LIMIT_CLEANUP_INTERVAL":      cfg.RateLimit.CleanupInterval.Seconds(),
	})...)
	return cfg, errors.Join(errs...)
}

func positive(values map[string]float64) []error {
	var errs []error
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if values[key] <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive", key))
		}
	}
	return errs
}

func str(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return defaultValue
}

func parse[T any](key string, defaultValue T, errs *[]error, fn func(string) (T, error)) T {
	v := str(key, "")
	if v == "" {
		return defaultValue
	}
	x, err := fn(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return defaultValue
	}
	return x
}

func integer(key string, defaultValue int, errs *[]error) int {
	return parse(key, defaultValue, errs, strconv.Atoi)
}

func float(key string, defaultValue float64, errs *[]error) float64 {
	return parse(key, defaultValue, errs, func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
}

func dur(key string, defaultValue time.Duration, errs *[]error) time.Duration {
	return parse(key, defaultValue, errs, time.ParseDuration)
}

func boolean(key string, defaultValue bool, errs *[]error) bool {
	return parse(key, defaultValue, errs, strconv.ParseBool)
}
