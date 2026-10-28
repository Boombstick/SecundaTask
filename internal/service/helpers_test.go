package service

import "taskservice/internal/platform/apperr"

func ptr(v int64) *int64 { return &v }

func kindOf(err error) apperr.Kind {
	if err == nil {
		return 0
	}
	e, ok := apperr.As(err)
	if !ok {
		return -1
	}
	return e.Kind
}
