package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/config"
	jwtlib "github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

func newDeviceIdentityTestService(t *testing.T) (*DeviceIdentityService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DeviceIdentity{}, &model.UserToken{}, &model.LoginLog{}); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	svc, err := NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: key, FeishuOidcOp: "feishu", EnterpriseClientType: "enterprise-windows"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, db
}

func allocateIdentity(t *testing.T, svc *DeviceIdentityService, db *gorm.DB, userID uint, machineUUID string) (*model.DeviceIdentity, string) {
	t.Helper()
	identity, credential, err := svc.AllocateOrGetDeviceIdentity(db, userID, machineUUID, DeviceInfo{OS: "windows", ClientType: "enterprise-windows"})
	if err != nil {
		t.Fatal(err)
	}
	return identity, credential
}

func TestAllocateOrGetDeviceIdentityCreatesAndReusesSamePair(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	first, firstCredential := allocateIdentity(t, svc, db, 7, "machine-a")
	second, secondCredential := allocateIdentity(t, svc, db, 7, "machine-a")
	if first.Id != second.Id || first.RustdeskId != second.RustdeskId || first.AllocationUuid != second.AllocationUuid {
		t.Fatalf("same identity pair was not reused: first=%+v second=%+v", first, second)
	}
	if firstCredential == "" || firstCredential != secondCredential {
		t.Fatalf("credential was not stably recovered")
	}
}

func TestAllocateOrGetDeviceIdentitySeparatesUsersAndMachines(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	a, _ := allocateIdentity(t, svc, db, 1, "machine-a")
	b, _ := allocateIdentity(t, svc, db, 1, "machine-b")
	c, _ := allocateIdentity(t, svc, db, 2, "machine-a")
	if a.RustdeskId == b.RustdeskId || a.RustdeskId == c.RustdeskId || b.RustdeskId == c.RustdeskId {
		t.Fatalf("distinct account/device pairs reused a RustDesk ID: %q %q %q", a.RustdeskId, b.RustdeskId, c.RustdeskId)
	}
}

func TestAllocateOrGetDeviceIdentityRejectsArchivedIdentity(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _ := allocateIdentity(t, svc, db, 7, "machine-a")
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, "machine-a", DeviceInfo{OS: "windows", ClientType: "enterprise-windows"})
	if !errors.Is(err, ErrDeviceIdentityArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
}

func TestAllocateOrGetDeviceIdentityRejectsEmptyMachineUUID(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	_, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, "", DeviceInfo{OS: "windows", ClientType: "enterprise-windows"})
	if !errors.Is(err, ErrMachineUUIDRequired) {
		t.Fatalf("expected machine uuid error, got %v", err)
	}
}

func TestAllocateOrGetDeviceIdentityConcurrentSamePairCreatesOneRow(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const workers = 100
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity, _, err := svc.AllocateOrGetDeviceIdentity(db, 9, "machine-concurrent", DeviceInfo{OS: "windows", ClientType: "enterprise-windows"})
			if err != nil {
				errs <- err
				return
			}
			ids <- identity.RustdeskId
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent allocation failed: %v", err)
	}
	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Fatalf("same pair returned different IDs: %q and %q", expected, id)
		}
	}
	var count int64
	if err := db.Model(&model.DeviceIdentity{}).Where("user_id = ? AND machine_uuid = ?", 9, "machine-concurrent").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one identity row, got %d", count)
	}
}

func TestDeviceCredentialFailsWithWrongMasterKey(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _ := allocateIdentity(t, svc, db, 7, "machine-a")
	wrongKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x24}, 32))
	wrongSvc, err := NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: wrongKey, FeishuOidcOp: "feishu", EnterpriseClientType: "enterprise-windows"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongSvc.DecryptCredential(identity); err == nil {
		t.Fatal("credential decrypted with the wrong master key")
	}
}

func TestNewDeviceIdentityServiceRejectsInvalidMasterKey(t *testing.T) {
	if _, err := NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: "not-a-key", FeishuOidcOp: "feishu", EnterpriseClientType: "enterprise-windows"}); err == nil {
		t.Fatal("invalid master key was accepted")
	}
}

func TestNewDeviceIdentityServiceRejectsMissingEnterpriseConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	if _, err := NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: key, EnterpriseClientType: "enterprise-windows"}); err == nil {
		t.Fatal("missing Feishu OIDC op was accepted")
	}
	if _, err := NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: key, FeishuOidcOp: "feishu"}); err == nil {
		t.Fatal("missing enterprise client type was accepted")
	}
}

func TestAllocateOrGetDeviceIdentityProducesGloballyUniqueIDs(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	seenIDs := map[string]bool{}
	seenAllocations := map[string]bool{}
	for i := 0; i < 100; i++ {
		identity, _ := allocateIdentity(t, svc, db, uint(i+1), "machine")
		if seenIDs[identity.RustdeskId] {
			t.Fatalf("duplicate RustDesk ID: %s", identity.RustdeskId)
		}
		if seenAllocations[identity.AllocationUuid] {
			t.Fatalf("duplicate allocation UUID: %s", identity.AllocationUuid)
		}
		seenIDs[identity.RustdeskId] = true
		seenAllocations[identity.AllocationUuid] = true
	}
}

func TestLoginWithDeviceIdentityRollsBackTokenWhenIdentityIsArchived(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = testOpaqueMachineUUID
	identity, _ := allocateIdentity(t, svc, db, 7, machine)
	if err := db.Model(identity).Update("status", model.DeviceIdentityStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldIdentityService, oldConfig, oldJWT := DB, AllService, Config, Jwt
	DB = db
	Config = &config.Config{}
	Jwt = jwtlib.NewJwt("", 0)
	AllService = &Service{DeviceIdentityService: svc}
	t.Cleanup(func() { DB, AllService, Config, Jwt = oldDB, oldIdentityService, oldConfig, oldJWT })
	user := &model.User{IdModel: model.IdModel{Id: 7}, Username: "feishu-user"}
	_, _, _, err := svc.LoginWithDeviceIdentity(user, &model.LoginLog{UserId: 7, Uuid: machine, DeviceId: "", Client: "enterprise-windows", Platform: "windows"})
	if !errors.Is(err, ErrDeviceIdentityArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
	var count int64
	if err := db.Model(&model.UserToken{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("token escaped failed transaction: %d rows", count)
	}
}

func TestLoginWithDeviceIdentityCommitsMatchingTokenAndLog(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	oldDB, oldServices, oldConfig, oldJWT := DB, AllService, Config, Jwt
	DB = db
	Config = &config.Config{}
	Jwt = jwtlib.NewJwt("", 0)
	AllService = &Service{DeviceIdentityService: svc}
	t.Cleanup(func() { DB, AllService, Config, Jwt = oldDB, oldServices, oldConfig, oldJWT })
	user := &model.User{IdModel: model.IdModel{Id: 7}, Username: "feishu-user"}
	token, identity, credential, err := svc.LoginWithDeviceIdentity(user, &model.LoginLog{UserId: 7, Uuid: testOpaqueMachineUUID, Client: "enterprise-windows", Platform: "windows"})
	if err != nil {
		t.Fatal(err)
	}
	if token.DeviceId != identity.RustdeskId || token.DeviceUuid != testOpaqueMachineUUID || !token.Managed || credential == "" {
		t.Fatalf("inconsistent login result: token=%+v identity=%+v", token, identity)
	}
	var log model.LoginLog
	if err := db.First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.UserTokenId != token.Id || log.DeviceId != identity.RustdeskId {
		t.Fatalf("login log not linked to token and identity: %+v", log)
	}
}

func TestLoginWithDeviceIdentityMakesOtherAccountOnMachineInactive(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	const machine = testOpaqueMachineUUID
	first, _ := allocateIdentity(t, svc, db, 7, machine)
	oldDB, oldServices, oldConfig, oldJWT := DB, AllService, Config, Jwt
	DB = db
	Config = &config.Config{}
	Jwt = jwtlib.NewJwt("", 0)
	AllService = &Service{DeviceIdentityService: svc}
	t.Cleanup(func() { DB, AllService, Config, Jwt = oldDB, oldServices, oldConfig, oldJWT })
	user := &model.User{IdModel: model.IdModel{Id: 8}, Username: "second-user"}
	if _, _, _, err := svc.LoginWithDeviceIdentity(user, &model.LoginLog{UserId: 8, Uuid: machine, Client: "enterprise-windows", Platform: "windows"}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(first, first.Id).Error; err != nil {
		t.Fatal(err)
	}
	if first.Status != model.DeviceIdentityStatusInactive {
		t.Fatalf("previous account identity remained %q", first.Status)
	}
}

func TestLoginWithDeviceIdentityCanonicalizesUUIDAndRevokesOtherAccountTokens(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	oldDB, oldServices, oldConfig, oldJWT := DB, AllService, Config, Jwt
	DB = db
	Config = &config.Config{}
	Jwt = jwtlib.NewJwt("", 0)
	AllService = &Service{DeviceIdentityService: svc}
	t.Cleanup(func() { DB, AllService, Config, Jwt = oldDB, oldServices, oldConfig, oldJWT })

	const canonical = testOpaqueMachineUUID
	if err := db.Create(&model.UserToken{UserId: 7, DeviceUuid: canonical, DeviceId: "old-id", Token: "old-token"}).Error; err != nil {
		t.Fatal(err)
	}
	user := &model.User{IdModel: model.IdModel{Id: 8}, Username: "second-user"}
	token, identity, _, err := svc.LoginWithDeviceIdentity(user, &model.LoginLog{UserId: 8, Uuid: "  " + canonical + "  ", Client: "enterprise-windows", Platform: "windows"})
	if err != nil {
		t.Fatal(err)
	}
	if token.DeviceUuid != canonical || identity.MachineUuid != canonical {
		t.Fatalf("UUID was not canonicalized: token=%q identity=%q", token.DeviceUuid, identity.MachineUuid)
	}
	var count int64
	if err := db.Model(&model.UserToken{}).Where("token = ?", "old-token").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("other account token for the same machine was not revoked")
	}
}
