package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/platform/apperr"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
} //@name ErrorResponse

type ErrorBody struct {
	// Machine-readable code: validation_error, unauthorized, forbidden,
	// team_not_found, task_not_found, version_conflict, rate_limited, ...
	Code string `json:"code" example:"validation_error"`
	// Human-readable text; for validation_error all messages joined with "; ".
	Message string `json:"message" example:"title is required; status is required"`
	// Every validation problem of the request (validation_error only).
	Details []FieldError `json:"details,omitempty"`
} //@name ErrorBody

type FieldError struct {
	Field   string `json:"field" example:"title"`
	Message string `json:"message" example:"title is required"`
} //@name FieldError

func MapSlice[S, D any](src []S, f func(S) D) []D {
	out := make([]D, len(src))
	for i, s := range src {
		out[i] = f(s)
	}
	return out
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func WriteErrorCode(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: msg}})
}

func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if e, ok := apperr.As(err); ok {
		status := map[apperr.Kind]int{
			apperr.KindValidation:   http.StatusBadRequest,
			apperr.KindUnauthorized: http.StatusUnauthorized,
			apperr.KindForbidden:    http.StatusForbidden,
			apperr.KindNotFound:     http.StatusNotFound,
			apperr.KindConflict:     http.StatusConflict,
		}[e.Kind]
		if status == 0 {
			status = http.StatusInternalServerError
		}
		body := ErrorBody{Code: e.Code, Message: e.Message}
		if len(e.Details) > 0 {
			body.Details = MapSlice(e.Details, func(d apperr.FieldError) FieldError {
				return FieldError{Field: d.Field, Message: d.Message}
			})
		}
		WriteJSON(w, status, ErrorResponse{Error: body})
		return
	}
	log.ErrorContext(r.Context(), "internal error", "error", err, "path", r.URL.Path)
	WriteErrorCode(w, http.StatusInternalServerError, "internal_error", "internal server error")
}

func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			return apperr.Validation("request body is too large")
		case errors.Is(err, io.EOF):
			return apperr.Validation("request body is empty")
		case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
			return apperr.Validation("request body is not valid JSON")
		case errors.As(err, &typeErr):
			return apperr.ValidationField(typeErr.Field, "field "+typeErr.Field+" has invalid type")
		default:
			if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
				name = strings.Trim(name, `"`)
				return apperr.ValidationField(name, "field "+name+" is not allowed")
			}
			return apperr.Validation(err.Error())
		}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return apperr.Validation("request body must contain a single JSON object")
	}
	return nil
}

func PathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.ValidationField(name, name+" must be a positive integer")
	}
	return id, nil
}
