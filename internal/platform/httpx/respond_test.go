package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"taskservice/internal/platform/apperr"
)

func TestDecodeJSON(t *testing.T) {
	type body struct {
		Title string `json:"title"`
	}
	decode := func(raw string) error {
		var dst body
		return DecodeJSON(httptest.NewRequest("POST", "/", strings.NewReader(raw)), &dst)
	}

	if err := decode(`{"title":"ok"}` + "\n"); err != nil {
		t.Fatalf("valid body: %v", err)
	}
	if err := decode(`{"title":"a"}{"title":"b"}`); !apperr.IsKind(err, apperr.KindValidation) {
		t.Errorf("trailing object must be rejected, got %v", err)
	}

	e, ok := apperr.As(decode(`{"title":"a","status":"done"}`))
	if !ok || len(e.Details) != 1 || e.Details[0].Field != "status" {
		t.Errorf("unknown field must be reported on its name, got %+v", e)
	}
}
