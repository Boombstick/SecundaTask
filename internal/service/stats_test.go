package service

import (
	"testing"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

func TestCanViewStats(t *testing.T) {
	cases := []struct {
		actor domain.Actor
		want  apperr.Kind
	}{
		{owner, 0},
		{admin, 0},
		{member, apperr.KindForbidden},
	}
	for _, c := range cases {
		if got := kindOf(CanViewStats(c.actor)); got != c.want {
			t.Errorf("role %s: got kind %d, want %d", c.actor.Role, got, c.want)
		}
	}
}
