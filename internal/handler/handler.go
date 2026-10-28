package handler

import (
	"log/slog"
	"net/http"

	"taskservice/internal/platform/httpx"
)

type base struct {
	log *slog.Logger
}

func (b base) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, b.log, err)
}
