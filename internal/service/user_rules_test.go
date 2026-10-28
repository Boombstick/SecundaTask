package service

import (
	"reflect"
	"strings"
	"testing"

	"taskservice/internal/platform/apperr"
)

func TestValidatePersonName(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"Alice", true},
		{"Анна-Мария", true},
		{"Jean-Luc O'Neil", true},
		{"J. R. R. Tolkien", true},
		{"Zoë", true},
		{"A", false},
		{"", false},
		{"1Alice", false},
		{"-Alice", false},
		{"Alice2", false},
		{"Alice<script>", false},
		{"Alice  Smith", false},
		{"Alice\tSmith", false},
		{strings.Repeat("a", 256), true},
		{strings.Repeat("a", 257), false},
		{strings.Repeat("я", 256), true},
	}
	for _, c := range cases {
		if err := ValidatePersonName(c.name); (err == nil) != c.valid {
			t.Errorf("ValidatePersonName(%q): err=%v, want valid=%v", c.name, err, c.valid)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	long := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 187) + ".com"
	cases := []struct {
		email string
		valid bool
	}{
		{"alice@example.com", true},
		{"alice.smith+tag@mail.example.org", true},
		{long, true},
		{long + "m", false},
		{strings.Repeat("a", 65) + "@example.com", false},
		{"", false},
		{"alice", false},
		{"alice@", false},
		{"@example.com", false},
		{"alice@localhost", false},
		{"alice@example.c", false},
		{"alice@example.123", false},
		{"Alice <alice@example.com>", false},
		{"alice @example.com", false},
		{"alice@exa mple.com", false},
	}
	for _, c := range cases {
		if err := ValidateEmail(c.email); (err == nil) != c.valid {
			t.Errorf("ValidateEmail(%q): err=%v, want valid=%v", c.email, err, c.valid)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		password string
		valid    bool
	}{
		{"Passw0rd", true},
		{"Пароль12", true},
		{"Pass0rd", false},
		{"password1", false},
		{"PASSWORD1", false},
		{"Password", false},
		{"Pass word1", false},
		{"Pass\x00word1", false},
		{"A1" + strings.Repeat("a", 70), true},
		{"A1" + strings.Repeat("a", 71), false},
		{"Я1" + strings.Repeat("я", 35), false},
	}
	for _, c := range cases {
		if err := ValidatePassword(c.password); (err == nil) != c.valid {
			t.Errorf("ValidatePassword(%q): err=%v, want valid=%v", c.password, err, c.valid)
		}
	}
}

func TestUserRulesReportAllProblems(t *testing.T) {
	details := func(err error) []string {
		e, ok := apperr.As(err)
		if !ok {
			t.Fatalf("want apperr.Error, got %v", err)
		}
		out := make([]string, len(e.Details))
		for i, d := range e.Details {
			out[i] = d.Field + ": " + d.Message
		}
		return out
	}
	got := details(ValidatePassword("пароль"))
	want := []string{
		"password: password must be at least 8 characters long",
		"password: password must contain an uppercase letter",
		"password: password must contain a digit",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("password details:\n got  %q\n want %q", got, want)
	}
	got = details(ValidatePersonName("1 bob  2"))
	want = []string{
		"name: name must start with a letter",
		"name: name may contain only letters, spaces, hyphens, apostrophes and dots",
		"name: name must not contain consecutive spaces",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("name details:\n got  %q\n want %q", got, want)
	}
}
