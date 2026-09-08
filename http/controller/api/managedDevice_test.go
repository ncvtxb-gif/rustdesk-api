package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

func newControllerDeviceIdentityService(t *testing.T) (*service.DeviceIdentityService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DeviceIdentity{}, &model.UserToken{}); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	svc, err := service.NewDeviceIdentityService(&config.DeviceIdentity{MasterKey: key, FeishuOidcOp: "feishu", EnterpriseClientType: "enterprise-windows"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, db
}

func TestSetManagedDeviceAuthenticationHashUsesAuthenticatedTokenIdentity(t *testing.T) {
	identityService, db := newControllerDeviceIdentityService(t)
	const machine = "550e8400-e29b-41d4-a716-446655440000"
	identity, _, err := identityService.AllocateOrGetDeviceIdentity(db, 7, machine, service.DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	token := &model.UserToken{UserId: 7, DeviceUuid: machine, DeviceId: identity.RustdeskId, Token: "access-token", ExpiredAt: 4102444800}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	oldDB, oldServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{UserService: &service.UserService{}, DeviceIdentityService: identityService}
	t.Cleanup(func() { service.DB, service.AllService = oldDB, oldServices })

	digest := sha256.Sum256([]byte("credential-and-device-salt"))
	wireHash := base64.StdEncoding.EncodeToString(digest[:])
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("curUser", &model.User{IdModel: model.IdModel{Id: 7}})
		c.Set("token", "access-token")
	})
	router.PUT("/api/managed-device/auth-hash", (&ManagedDevice{}).SetAuthenticationHash)
	req := httptest.NewRequest(http.MethodPut, "/api/managed-device/auth-hash", bytes.NewBufferString(`{"hash":"`+wireHash+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := identityService.AuthenticationHashByRustdeskID(db, identity.RustdeskId); got != wireHash {
		t.Fatalf("stored hash %q, want %q", got, wireHash)
	}
}

func TestManagedDeviceBootstrapReturnsOnlyAuthenticatedTokenIdentity(t *testing.T) {
	identityService, db := newControllerDeviceIdentityService(t)
	const machine = "550e8400-e29b-41d4-a716-446655440000"
	identity, credential, err := identityService.AllocateOrGetDeviceIdentity(db, 7, machine, service.DeviceInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserToken{UserId: 7, DeviceUuid: machine, DeviceId: identity.RustdeskId, Token: "access-token", ExpiredAt: 4102444800}).Error; err != nil {
		t.Fatal(err)
	}

	oldDB, oldServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{UserService: &service.UserService{}, DeviceIdentityService: identityService}
	t.Cleanup(func() { service.DB, service.AllService = oldDB, oldServices })
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("curUser", &model.User{IdModel: model.IdModel{Id: 7}})
		c.Set("token", "access-token")
	})
	router.POST("/api/managed-device/bootstrap", (&ManagedDevice{}).Bootstrap)
	req := httptest.NewRequest(http.MethodPost, "/api/managed-device/bootstrap", bytes.NewBufferString(`{"machine_uuid":"{550E8400-E29B-41D4-A716-446655440000}","platform":"windows"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{identity.RustdeskId, credential, machine, `"status":"active"`} {
		if !bytes.Contains(recorder.Body.Bytes(), []byte(want)) {
			t.Fatalf("bootstrap response %q missing %q", recorder.Body.String(), want)
		}
	}
}
