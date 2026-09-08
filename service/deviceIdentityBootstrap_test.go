package service

import (
	"errors"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

const testOpaqueMachineUUID = "YTFhMmIzYzQtZDVlNi00ZjcwLTgxOTItYTNkNGU1ZjYwNzE4"

func TestNormalizeMachineUUIDAcceptsRustDeskOpaqueBase64AndPreservesCase(t *testing.T) {
	got, err := NormalizeMachineUUID("  " + testOpaqueMachineUUID + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != testOpaqueMachineUUID {
		t.Fatalf("normalized UUID %q, want %q", got, testOpaqueMachineUUID)
	}
	if _, err := NormalizeMachineUUID("not base64!"); err == nil {
		t.Fatal("expected malformed base64 UUID rejection")
	}
}

func TestBootstrapRequiresTokenDeviceAndActiveIdentityMatch(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = testOpaqueMachineUUID
	identity, credential, err := svc.AllocateOrGetDeviceIdentity(db, 7, machine, DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	token := &model.UserToken{UserId: 7, DeviceUuid: machine, DeviceId: identity.RustdeskId, Token: "access-token", ExpiredAt: 4102444800}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	got, gotCredential, expiresAt, err := svc.BootstrapForToken(db, 7, "access-token", "  "+machine+"  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Id != identity.Id || gotCredential != credential || got.MachineUuid != machine || expiresAt != 4102444800 {
		t.Fatalf("unexpected bootstrap identity: %+v", got)
	}
	if _, _, _, err := svc.BootstrapForToken(db, 7, "access-token", "YjJiMmMzZDQtZTVmNi00YTcwLTgxOTItYTNkNGU1ZjYwNzE4"); !errors.Is(err, ErrManagedDeviceUnauthorized) {
		t.Fatalf("expected machine mismatch rejection, got %v", err)
	}
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusInactive).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.BootstrapForToken(db, 7, "access-token", machine); !errors.Is(err, ErrManagedDeviceUnauthorized) {
		t.Fatalf("expected inactive rejection, got %v", err)
	}
}

func TestSetAuthenticationHashForTokenRejectsMismatchedDevice(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = testOpaqueMachineUUID
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
