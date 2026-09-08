package service

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestSetAuthenticationHashStoresOnlyCompatibleDerivedHash(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, credential := allocateIdentity(t, svc, db, 7, "machine-a")
	digest := sha256.Sum256([]byte(credential + "device-salt"))
	wireHash := base64.StdEncoding.EncodeToString(digest[:])
	if err := svc.SetAuthenticationHash(db, 7, "machine-a", wireHash); err != nil {
		t.Fatal(err)
	}
	var stored model.DeviceIdentity
	if err := db.First(&stored, identity.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AuthenticationHash != wireHash {
		t.Fatalf("stored hash mismatch: %q", stored.AuthenticationHash)
	}
	if stored.AuthenticationHash == credential {
		t.Fatal("plaintext credential was persisted as authentication hash")
	}
}

func TestSetAuthenticationHashRejectsMalformedAndArchivedIdentity(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _ := allocateIdentity(t, svc, db, 7, "machine-a")
	if err := svc.SetAuthenticationHash(db, 7, "machine-a", base64.StdEncoding.EncodeToString([]byte("short"))); !errors.Is(err, ErrInvalidAuthenticationHash) {
		t.Fatalf("expected malformed hash rejection, got %v", err)
	}
	if err := svc.SetAuthenticationHash(db, 7, "machine-a", string(make([]byte, 1<<20))); !errors.Is(err, ErrInvalidAuthenticationHash) {
		t.Fatalf("expected oversized hash rejection, got %v", err)
	}
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("valid derived material"))
	if err := svc.SetAuthenticationHash(db, 7, "machine-a", base64.StdEncoding.EncodeToString(digest[:])); !errors.Is(err, ErrDeviceIdentityArchived) {
		t.Fatalf("expected archived rejection, got %v", err)
	}
}

func TestAuthenticationHashByRustdeskIDReturnsActiveIdentityOnly(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _ := allocateIdentity(t, svc, db, 7, "machine-a")
	digest := sha256.Sum256([]byte("derived"))
	wireHash := base64.StdEncoding.EncodeToString(digest[:])
	if err := svc.SetAuthenticationHash(db, 7, "machine-a", wireHash); err != nil {
		t.Fatal(err)
	}
	if got := svc.AuthenticationHashByRustdeskID(db, identity.RustdeskId); got != wireHash {
		t.Fatalf("got %q, want %q", got, wireHash)
	}
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	if got := svc.AuthenticationHashByRustdeskID(db, identity.RustdeskId); got != "" {
		t.Fatalf("archived identity leaked hash %q", got)
	}
}
