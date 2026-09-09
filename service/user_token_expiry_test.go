package service

import (
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
)

func TestUserTokenExpireTimestampDefaultsToSevenDays(t *testing.T) {
	oldConfig := Config
	Config = &config.Config{}
	t.Cleanup(func() { Config = oldConfig })

	before := time.Now().Add(7 * 24 * time.Hour).Unix()
	got := (&UserService{}).UserTokenExpireTimestamp()
	after := time.Now().Add(7 * 24 * time.Hour).Unix()
	if got < before || got > after {
		t.Fatalf("default expiry = %d, want between %d and %d", got, before, after)
	}
}
