package service

import (
	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

func CanViewStats(a domain.Actor) error {
	if !a.Role.IsManager() {
		return apperr.Forbidden("only the team owner or an admin can view team stats")
	}
	return nil
}
