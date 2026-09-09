package service

import "testing"

func TestOAuthRedirectURLPreservesConfiguredHTTPS(t *testing.T) {
	got := oauthRedirectURL("https://api.example.com")
	want := "https://api.example.com/api/oidc/callback"
	if got != want {
		t.Fatalf("oauthRedirectURL() = %q, want %q", got, want)
	}
}
