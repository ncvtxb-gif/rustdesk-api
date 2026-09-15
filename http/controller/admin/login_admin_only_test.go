package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/gorm"
)

func TestResponseLoginSuccessRejectsOrdinaryUserAndRevokesToken(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserToken{}); err != nil {
		t.Fatal(err)
	}

	oldDB, oldServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{UserService: &service.UserService{}}
	t.Cleanup(func() {
		service.DB = oldDB
		service.AllService = oldServices
	})

	admin := false
	user := &model.User{IsAdmin: &admin}
	token := &model.UserToken{UserId: 42, Token: "ordinary-web-token"}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	responseLoginSuccess(ctx, user, token.Token)

	var payload response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != 403 {
		t.Fatalf("response code=%d; want 403", payload.Code)
	}

	var count int64
	if err := db.Model(&model.UserToken{}).Where("token = ?", token.Token).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ordinary admin-portal token remains usable; count=%d", count)
	}
}

func TestResponseLoginSuccessAllowsAdministrator(t *testing.T) {
	oldServices := service.AllService
	service.AllService = &service.Service{UserService: &service.UserService{}}
	t.Cleanup(func() { service.AllService = oldServices })

	admin := true
	user := &model.User{Username: "admin", IsAdmin: &admin}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	responseLoginSuccess(ctx, user, "admin-web-token")

	var payload response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != 0 {
		t.Fatalf("response code=%d; want success", payload.Code)
	}
}
