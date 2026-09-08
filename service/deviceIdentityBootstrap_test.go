package service

import (
	"errors"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestBootstrapRequiresTokenDeviceAndActiveIdentityMatch(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = "550e8400-e29b-41d4-a716-446655440000"
	identity, credential, err := svc.AllocateOrGetDeviceIdentity(db, 7, machine, DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	token := &model.UserToken{UserId: 7, DeviceUuid: machine, DeviceId: identity.RustdeskId, Token: "access-token", ExpiredAt: 4102444800}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	got, gotCredential, err := svc.BootstrapForToken(db, 7, "access-token", "{550E8400-E29B-41D4-A716-446655440000}")
	if err != nil {
		t.Fatal(err)
	}
	if got.Id != identity.Id || gotCredential != credential || got.MachineUuid != machine {
		t.Fatalf("unexpected bootstrap identity: %+v", got)
	}
	if _, _, err := svc.BootstrapForToken(db, 7, "access-token", "550e8400-e29b-41d4-a716-446655440001"); !errors.Is(err, ErrManagedDeviceUnauthorized) {
		t.Fatalf("expected machine mismatch rejection, got %v", err)
	}
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusInactive).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.BootstrapForToken(db, 7, "access-token", machine); !errors.Is(err, ErrManagedDeviceUnauthorized) {
		t.Fatalf("expected inactive rejection, got %v", err)
	}
}

func TestSetAuthenticationHashForTokenRejectsMismatchedDevice(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = "550e8400-e29b-41d4-a716-446655440000"
	identity, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, machine, DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserToken{UserId: 7, DeviceUuid: machine, DeviceId: "different-id", Token: "access-token"}).Error; err != nil {
		t.Fatal(err)
	}
	validHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := svc.SetAuthenticationHashForToken(db, 7, "access-token", validHash); !errors.Is(err, ErrManagedDeviceUnauthorized) {
		t.Fatalf("expected token device mismatch rejection, got %v", err)
	}
	if got := svc.AuthenticationHashByRustdeskID(db, identity.RustdeskId); got != "" {
		t.Fatalf("mismatched token stored hash %q", got)
	}
}
