package api

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestLoginUsernameLengthAllowsExternalIdentityIdentifier(t *testing.T) {
	validate := validator.New()
	form := LoginForm{Username: strings.Repeat("u", 128), Password: "pass"}

	if err := validate.Struct(form); err != nil {
		t.Fatalf("128-character username should be accepted: %v", err)
	}
}

func TestLoginUsernameLengthRejectsMoreThan128Characters(t *testing.T) {
	validate := validator.New()
	form := LoginForm{Username: strings.Repeat("u", 129), Password: "pass"}

	if err := validate.Struct(form); err == nil {
		t.Fatal("129-character username should be rejected")
	}
}
