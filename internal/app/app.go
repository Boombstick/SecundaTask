package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"taskservice/internal/cache"
	"taskservice/internal/config"
	"taskservice/internal/handler"
	"taskservice/internal/platform/database"
	"taskservice/internal/platform/httpx"
	"taskservice/internal/repository"
	"taskservice/internal/service"
)

func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if cfg.MigrateOnStart {
		if err := retry(sigCtx, log, cfg.Startup, "migrate", func() error { return database.Migrate(cfg.MySQL.MigrateDSN()) }); err != nil {
			return startupErr(err)
		}
		log.Info("migrations applied")
	}

	var db *database.DB
	if err := retry(sigCtx, log, cfg.Startup, "mysql", func() error {
		var err error
		db, err = database.Open(sigCtx, cfg.MySQL.DSN(), database.Pool{
			MaxOpenConns:    cfg.MySQL.MaxOpenConns,
			MaxIdleConns:    cfg.MySQL.MaxIdleConns,
			ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
		})
		return err
	}); err != nil {
		return startupErr(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	if err := rdb.Ping(sigCtx).Err(); err != nil {
		log.Warn("redis is unavailable", "error", err)
	}
	taskCache := cache.NewTaskList(rdb, cache.Options{
		TTL:                     cfg.Cache.TTL,
		InvalidateTimeout:       cfg.Cache.InvalidateTimeout,
		InvalidateRetryInterval: cfg.Cache.InvalidateRetryInterval,
		BreakerFailures:         cfg.Cache.BreakerFailures,
		BreakerOpenTimeout:      cfg.Cache.BreakerOpenTimeout,
		BreakerHalfOpenRequests: cfg.Cache.BreakerHalfOpenRequests,
	}, log)

	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	members := service.NewMembership(teams)

	authSvc := service.NewAuthService(users, repository.NewRefreshTokenRepository(db), db,
		cfg.Auth.JWTSecret, cfg.Auth.AccessTTL, cfg.Auth.RefreshTTL)
	history := repository.NewHistoryRepository(db)
	taskSvc := service.NewTaskService(repository.NewTaskRepository(db), history, members, taskCache, db)
	teamSvc := service.NewTeamService(teams, members, users, taskSvc, taskCache, db)
	commentSvc := service.NewCommentService(repository.NewCommentRepository(db), taskSvc)
	historySvc := service.NewHistoryService(history, taskSvc)
	statsSvc := service.NewStatsService(repository.NewStatsRepository(db), members)

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	go taskCache.Run(appCtx)

	deps := routerDeps{
		log:       log,
		auth:      authSvc,
		pingMySQL: db.Ping,
		pingRedis: func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
		rateLimit: httpx.RateLimitOptions{
			RPS:               cfg.RateLimit.RPS,
			Burst:             cfg.RateLimit.Burst,
			TrustProxyHeaders: cfg.HTTP.TrustProxyHeaders,
			ClientTTL:         cfg.RateLimit.ClientTTL,
			CleanupInterval:   cfg.RateLimit.CleanupInterval,
		},
		trustProxyHeaders: cfg.HTTP.TrustProxyHeaders,
		maxBodyBytes:      cfg.HTTP.MaxBodyBytes,
	}
	deps.handlers.auth = handler.NewAuthHandler(authSvc, log)
	deps.handlers.team = handler.NewTeamHandler(teamSvc, log)
	deps.handlers.task = handler.NewTaskHandler(taskSvc, log)
	deps.handlers.history = handler.NewHistoryHandler(historySvc, log)
	deps.handlers.comment = handler.NewCommentHandler(commentSvc, log)
	deps.handlers.stats = handler.NewStatsHandler(statsSvc, log)

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           newRouter(appCtx, deps),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		return errors.Join(err, closeResources(log, db, rdb))
	case <-sigCtx.Done():
	}
	stopSignals()

	return shutdown(log, cfg, srv, cancelApp, db, rdb)
}

func shutdown(log *slog.Logger, cfg config.Config, srv *http.Server,
	cancelApp context.CancelFunc, db *database.DB, rdb *redis.Client) error {
	start := time.Now()
	log.Info("shutdown started", "timeout", cfg.HTTP.ShutdownTimeout.String())

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("in-flight requests did not finish in time, closing connections", "error", err)
		errs = append(errs, err, srv.Close())
	} else {
		log.Info("http server stopped, all in-flight requests completed")
	}

	cancelApp()
	errs = append(errs, closeResources(log, db, rdb))
	log.Info("shutdown complete", "duration_ms", time.Since(start).Milliseconds())
	return errors.Join(errs...)
}

func closeResources(log *slog.Logger, db *database.DB, rdb *redis.Client) error {
	var errs []error
	if err := db.Close(); err != nil {
		errs = append(errs, err)
	} else {
		log.Info("mysql connections closed")
	}
	if err := rdb.Close(); err != nil {
		errs = append(errs, err)
	} else {
		log.Info("redis connections closed")
	}
	return errors.Join(errs...)
}

func startupErr(err error) error {
	if errors.Is(err, context.Canceled) {
		slog.Info("startup interrupted by signal")
		return nil
	}
	return err
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

func retry(ctx context.Context, log *slog.Logger, opts config.Startup, name string, fn func() error) error {
	var err error
	for i := 1; i <= opts.RetryAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		log.Warn("dependency not ready, retrying", "dependency", name, "attempt", i, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.RetryInterval):
		}
	}
	return err
}
