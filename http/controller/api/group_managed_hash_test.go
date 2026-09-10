package api

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

func TestManagedAuthenticationHashRequiresAdministrator(t *testing.T) {
	identityService, db := newControllerDeviceIdentityService(t)
	identity, _, err := identityService.AllocateOrGetDeviceIdentity(db, 7, "machine-a", service.DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("derived"))
	wireHash := base64.StdEncoding.EncodeToString(digest[:])
	if err := identityService.SetAuthenticationHash(db, 7, "machine-a", wireHash); err != nil {
		t.Fatal(err)
	}
	oldDB, oldServices := service.DB, service.AllService
	service.DB = db
	// Production initializes the stateless embedded services as nil pointers.
	service.AllService = &service.Service{DeviceIdentityService: identityService}
	t.Cleanup(func() { service.DB, service.AllService = oldDB, oldServices })

	admin := true
	ordinary := false
	if got := managedAuthenticationHash(&model.User{}, identity.RustdeskId); got != "" {
		t.Fatalf("user without administrator flag received hash %q", got)
	}
	if got := managedAuthenticationHash(&model.User{IsAdmin: &ordinary}, identity.RustdeskId); got != "" {
		t.Fatalf("ordinary user received hash %q", got)
	}
	if got := managedAuthenticationHash(&model.User{IsAdmin: &admin}, identity.RustdeskId); got != wireHash {
		t.Fatalf("administrator got %q, want %q", got, wireHash)
	}
}
