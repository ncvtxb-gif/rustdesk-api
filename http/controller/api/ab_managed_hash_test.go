package api

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

func TestManagedAddressBookHashesAreOnlySuppliedToAdministrators(t *testing.T) {
	identityService, db := newControllerDeviceIdentityService(t)
	identity, _, err := identityService.AllocateOrGetDeviceIdentity(db, 7, "machine-a", service.DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("managed-address-book-credential"))
	wireHash := base64.StdEncoding.EncodeToString(digest[:])
	if err := identityService.SetAuthenticationHash(db, 7, "machine-a", wireHash); err != nil {
		t.Fatal(err)
	}

	oldDB, oldServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{UserService: &service.UserService{}, DeviceIdentityService: identityService}
	t.Cleanup(func() { service.DB, service.AllService = oldDB, oldServices })

	adminFlag, ordinaryFlag := true, false
	adminEntry := &model.AddressBook{Id: identity.RustdeskId}
	supplyManagedAddressBookHashes(&model.User{IsAdmin: &adminFlag}, []*model.AddressBook{adminEntry})
	if adminEntry.Hash != wireHash {
		t.Fatalf("administrator address-book entry got hash %q, want %q", adminEntry.Hash, wireHash)
	}

	ordinaryEntry := &model.AddressBook{Id: identity.RustdeskId}
	supplyManagedAddressBookHashes(&model.User{IsAdmin: &ordinaryFlag}, []*model.AddressBook{ordinaryEntry})
	if ordinaryEntry.Hash != "" {
		t.Fatalf("ordinary user received managed hash %q", ordinaryEntry.Hash)
	}
}
