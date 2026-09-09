package service

import (
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

func TestOauthAuthCodeOptionsForceReauthentication(t *testing.T) {
	config := &oauth2.Config{
		ClientID: "client-id",
		Endpoint: oauth2.Endpoint{AuthURL: "https://example.test/authorize"},
	}

	for _, tc := range []struct {
		name                  string
		forceReauthentication bool
		wantPrompt            string
	}{
		{name: "ordinary login keeps provider session behavior"},
		{name: "enterprise login requires fresh user authentication", forceReauthentication: true, wantPrompt: "login"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authURL := config.AuthCodeURL(
				"state",
				oauthAuthCodeOptions("nonce", tc.forceReauthentication)...,
			)
			parsed, err := url.Parse(authURL)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}

			if got := parsed.Query().Get("nonce"); got != "nonce" {
				t.Fatalf("nonce = %q, want nonce", got)
			}
			if got := parsed.Query().Get("prompt"); got != tc.wantPrompt {
				t.Fatalf("prompt = %q, want %q", got, tc.wantPrompt)
			}
		})
	}
}
