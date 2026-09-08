package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
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

func TestPeriodicBootstrapReturnsSlidingRenewalExpiry(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, testOpaqueMachineUUID, DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	oldExpiry := time.Now().Unix() + 60
	token := &model.UserToken{UserId: 7, DeviceUuid: testOpaqueMachineUUID, DeviceId: identity.RustdeskId, Token: "periodic-token", ExpiredAt: oldExpiry}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := DB, Config
	DB = db
	Config = &config.Config{App: config.App{TokenExpire: 24 * time.Hour}}
	t.Cleanup(func() { DB, Config = oldDB, oldConfig })

	(&UserService{}).AutoRefreshAccessToken(token)
	_, _, returnedExpiry, err := svc.BootstrapForToken(db, 7, token.Token, testOpaqueMachineUUID)
	if err != nil {
		t.Fatal(err)
	}
	var stored model.UserToken
	if err := db.Where("token = ?", token.Token).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if returnedExpiry != stored.ExpiredAt || returnedExpiry <= oldExpiry {
		t.Fatalf("bootstrap expiry=%d, stored=%d, old=%d", returnedExpiry, stored.ExpiredAt, oldExpiry)
	}
}

func TestConcurrentSlidingRenewalBootstrapReturnsCurrentExpiry(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	identity, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, testOpaqueMachineUUID, DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	token := &model.UserToken{UserId: 7, DeviceUuid: testOpaqueMachineUUID, DeviceId: identity.RustdeskId, Token: "concurrent-token", ExpiredAt: time.Now().Unix() + 60}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := DB, Config
	DB = db
	Config = &config.Config{App: config.App{TokenExpire: 24 * time.Hour}}
	t.Cleanup(func() { DB, Config = oldDB, oldConfig })

	const workers = 8
	start := make(chan struct{})
	expiries := make(chan int64, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var copy model.UserToken
			if err := db.Where("token = ?", token.Token).First(&copy).Error; err != nil {
				errs <- err
				return
			}
			(&UserService{}).AutoRefreshAccessToken(&copy)
			_, _, expiry, err := svc.BootstrapForToken(db, 7, token.Token, testOpaqueMachineUUID)
			if err != nil {
				errs <- err
				return
			}
			expiries <- expiry
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(expiries)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var stored model.UserToken
	if err := db.Where("token = ?", token.Token).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	for expiry := range expiries {
		if expiry != stored.ExpiredAt {
			t.Fatalf("concurrent bootstrap returned %d, current stored expiry is %d", expiry, stored.ExpiredAt)
		}
	}
}

func TestBootstrapRejectsInactiveAndArchivedEnterpriseIdentity(t *testing.T) {
	for _, status := range []string{model.DeviceIdentityStatusInactive, model.DeviceIdentityStatusArchived} {
		t.Run(status, func(t *testing.T) {
			svc, db := newDeviceIdentityTestService(t)
			identity, _, err := svc.AllocateOrGetDeviceIdentity(db, 7, testOpaqueMachineUUID, DeviceInfo{})
			if err != nil {
				t.Fatal(err)
			}
			oldExpiry := time.Now().Unix() + 60
			token := &model.UserToken{UserId: 7, DeviceUuid: testOpaqueMachineUUID, DeviceId: identity.RustdeskId, Token: "status-token", ExpiredAt: oldExpiry}
			if err := db.Create(token).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(identity).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
			oldDB, oldConfig := DB, Config
			DB = db
			Config = &config.Config{App: config.App{TokenExpire: 24 * time.Hour}}
			t.Cleanup(func() { DB, Config = oldDB, oldConfig })
			(&UserService{}).AutoRefreshAccessToken(token)
			var stored model.UserToken
			if err := db.Where("token = ?", token.Token).First(&stored).Error; err != nil {
				t.Fatal(err)
			}
			if stored.ExpiredAt != oldExpiry {
				t.Fatalf("%s enterprise identity renewed token from %d to %d", status, oldExpiry, stored.ExpiredAt)
			}
			if _, _, _, err := svc.BootstrapForToken(db, 7, "status-token", testOpaqueMachineUUID); !errors.Is(err, ErrManagedDeviceUnauthorized) {
				t.Fatalf("expected %s identity rejection, got %v", status, err)
			}
		})
	}
}

func TestManagedTokenWithoutIdentityDoesNotRefresh(t *testing.T) {
	_, db := newDeviceIdentityTestService(t)
	oldExpiry := time.Now().Unix() + 60
	token := &model.UserToken{UserId: 7, DeviceUuid: testOpaqueMachineUUID, DeviceId: "123456789", Token: "missing-managed-identity", ExpiredAt: oldExpiry}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.LoginLog{
		UserId: token.UserId, UserTokenId: token.Id, Client: "enterprise-windows",
		Platform: "windows", Type: model.LoginLogTypeOauth, Uuid: token.DeviceUuid, DeviceId: token.DeviceId,
	}).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := DB, Config
	DB = db
	Config = &config.Config{
		App:            config.App{TokenExpire: 24 * time.Hour},
		DeviceIdentity: config.DeviceIdentity{Enabled: true, EnterpriseClientType: "enterprise-windows"},
	}
	t.Cleanup(func() { DB, Config = oldDB, oldConfig })

	(&UserService{}).AutoRefreshAccessToken(token)
	var stored model.UserToken
	if err := db.First(&stored, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ExpiredAt != oldExpiry {
		t.Fatalf("managed token without identity renewed from %d to %d", oldExpiry, stored.ExpiredAt)
	}
}

func TestLegacyTokenWithoutIdentityRetainsSlidingRefresh(t *testing.T) {
	_, db := newDeviceIdentityTestService(t)
	oldExpiry := time.Now().Unix() + 60
	token := &model.UserToken{UserId: 7, DeviceUuid: testOpaqueMachineUUID, DeviceId: "legacy-device", Token: "legacy-token", ExpiredAt: oldExpiry}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := DB, Config
	DB = db
	Config = &config.Config{
		App:            config.App{TokenExpire: 24 * time.Hour},
		DeviceIdentity: config.DeviceIdentity{Enabled: true, EnterpriseClientType: "enterprise-windows"},
	}
	t.Cleanup(func() { DB, Config = oldDB, oldConfig })

	(&UserService{}).AutoRefreshAccessToken(token)
	var stored model.UserToken
	if err := db.First(&stored, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ExpiredAt <= oldExpiry {
		t.Fatalf("legacy token did not retain sliding refresh: old=%d stored=%d", oldExpiry, stored.ExpiredAt)
	}
}
