package admin

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestUsernameLengthAllowsExternalIdentityIdentifiers(t *testing.T) {
	validate := validator.New()

	for name, form := range map[string]interface{}{
		"edit":     UserForm{Username: strings.Repeat("u", 128), GroupId: 1, Status: 1},
		"register": RegisterForm{Username: strings.Repeat("u", 128), Password: "pass", ConfirmPassword: "pass"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate.Struct(form); err != nil {
				t.Fatalf("128-character username should be accepted: %v", err)
			}
		})
	}
}

func TestUsernameLengthRejectsMoreThan128Characters(t *testing.T) {
	validate := validator.New()

	for name, form := range map[string]interface{}{
		"edit":     UserForm{Username: strings.Repeat("u", 129), GroupId: 1, Status: 1},
		"register": RegisterForm{Username: strings.Repeat("u", 129), Password: "pass", ConfirmPassword: "pass"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validate.Struct(form); err == nil {
				t.Fatal("129-character username should be rejected")
			}
		})
	}
}
