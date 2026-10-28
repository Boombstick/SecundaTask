package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "taskservice/api"
	"taskservice/internal/handler"
	"taskservice/internal/platform/httpx"
	"taskservice/internal/service"
)

type routerDeps struct {
	log      *slog.Logger
	auth     *service.AuthService
	handlers struct {
		auth    *handler.AuthHandler
		team    *handler.TeamHandler
		task    *handler.TaskHandler
		history *handler.HistoryHandler
		comment *handler.CommentHandler
		stats   *handler.StatsHandler
	}

	pingMySQL pinger
	pingRedis pinger

	rateLimit         httpx.RateLimitOptions
	trustProxyHeaders bool
	maxBodyBytes      int64
}

type pinger func(ctx context.Context) error

func newRouter(ctx context.Context, d routerDeps) http.Handler {
	rl := httpx.NewRateLimiter(ctx, d.rateLimit)

	r := chi.NewRouter()
	r.Use(httpx.RequestID, httpx.AccessLog(d.log, d.trustProxyHeaders), httpx.Recoverer(d.log), httpx.LimitBody(d.maxBodyBytes))

	r.Get("/healthz", health(d.pingMySQL, d.pingRedis))

	r.Get("/swagger", http.RedirectHandler("/swagger/index.html", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(rl.Middleware)

		d.handlers.auth.Routes(r)

		r.Group(func(r chi.Router) {
			r.Use(httpx.Authenticate(d.auth))
			d.handlers.team.Routes(r)
			d.handlers.task.Routes(r)
			d.handlers.history.Routes(r)
			d.handlers.comment.Routes(r)
			d.handlers.stats.Routes(r)
		})
	})

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteErrorCode(w, http.StatusNotFound, "not_found", "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteErrorCode(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return r
}

func health(mysql, redis pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		status, res := http.StatusOK, map[string]string{"mysql": "ok", "redis": "ok"}
		if err := mysql(ctx); err != nil {
			status, res["mysql"] = http.StatusServiceUnavailable, "down"
		}
		if err := redis(ctx); err != nil {
			res["redis"] = "down"
		}
		httpx.WriteJSON(w, status, res)
	}
}
