// @title						Task Service API
// @version					1.0
// @description				REST API для управления задачами внутри команд: роли, история изменений, кеширование списков задач и SQL-отчёт.
// @description				Ошибки всегда приходят в формате {"error": {"code", "message", "details"}}; в details — все ошибки валидации запроса.
// @BasePath					/api/v1
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Access-токен из /login в виде "Bearer <token>".
package main

import (
	"log/slog"
	"os"

	"taskservice/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		slog.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}
