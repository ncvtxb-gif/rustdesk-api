package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

func setupOauthUserUniquenessTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserThird{}, &model.Oauth{}); err != nil {
		t.Fatal(err)
	}
	autoRegister := true
	if err := db.Create(&model.Oauth{Op: "feishu", OauthType: model.OauthTypeFeishu, AutoRegister: &autoRegister}).Error; err != nil {
		t.Fatal(err)
	}

	oldConfig, oldDB, oldLock, oldServices := Config, DB, Lock, AllService
	Config = &config.Config{}
	DB = db
	Lock = lock.NewLocal()
	AllService = &Service{
		UserService:  &UserService{},
		OauthService: &OauthService{},
		LdapService:  &LdapService{},
	}
	t.Cleanup(func() {
		Config, DB, Lock, AllService = oldConfig, oldDB, oldLock, oldServices
	})
	return db
}

func TestRegisterByOauthRejectsFeishuUserWithoutEmail(t *testing.T) {
	setupOauthUserUniquenessTest(t)

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId:   "ou_missing_email",
		Name:     "无邮箱用户",
		Username: "无邮箱用户",
	}, "feishu")
	if err == nil || err.Error() != "FeishuEmailRequired" || user != nil {
		t.Fatalf("RegisterByOauth = (%v, %#v), want (FeishuEmailRequired, nil)", err, user)
	}
}

func TestRegisterByOauthUsesNormalizedEmailBeforeExistingOpenIDBinding(t *testing.T) {
	db := setupOauthUserUniquenessTest(t)
	canonical := &model.User{Username: "canonical", Nickname: "李伟铭", Email: "lwm@sweetnight.com", GroupId: 1}
	shadow := &model.User{Username: "shadow", Nickname: "李伟铭", Email: "", GroupId: 1}
	if err := db.Create(canonical).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(shadow).Error; err != nil {
		t.Fatal(err)
	}
	binding := &model.UserThird{UserId: shadow.Id, OauthUser: model.OauthUser{OpenId: "ou_changed", Name: "李伟铭"}, OauthType: model.OauthTypeFeishu, Op: "feishu"}
	if err := db.Create(binding).Error; err != nil {
		t.Fatal(err)
	}

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId:   "ou_changed",
		Name:     "李伟铭",
		Username: "李伟铭",
		Email:    "  LWM@SweetNight.COM  ",
	}, "feishu")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id != canonical.Id {
		t.Fatalf("resolved user = %#v, want canonical user id %d", user, canonical.Id)
	}
	if err := db.First(binding, binding.Id).Error; err != nil {
		t.Fatal(err)
	}
	if binding.UserId != canonical.Id || binding.Email != "lwm@sweetnight.com" {
		t.Fatalf("binding = %#v, want canonical user and normalized email", binding)
	}
}

func TestRegisterByOauthBackfillsEmailForExistingFeishuBinding(t *testing.T) {
	db := setupOauthUserUniquenessTest(t)
	bound := &model.User{Username: "bound", Nickname: "旧姓名", Email: "", Avatar: "old-avatar", GroupId: 1}
	if err := db.Create(bound).Error; err != nil {
		t.Fatal(err)
	}
	binding := &model.UserThird{UserId: bound.Id, OauthUser: model.OauthUser{OpenId: "ou_existing"}, OauthType: model.OauthTypeFeishu, Op: "feishu"}
	if err := db.Create(binding).Error; err != nil {
		t.Fatal(err)
	}

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId:  "ou_existing",
		Name:    "新姓名",
		Email:   " NEW@SweetNight.COM ",
		Picture: "new-avatar",
	}, "feishu")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id != bound.Id {
		t.Fatalf("resolved user = %#v, want bound user id %d", user, bound.Id)
	}
	if err := db.First(bound, bound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if bound.Email != "new@sweetnight.com" || bound.Nickname != "新姓名" || bound.Avatar != "new-avatar" {
		t.Fatalf("bound user was not refreshed: %#v", bound)
	}
}

func TestRegisterByOauthReusesUserForDifferentOpenIDWithSameEmail(t *testing.T) {
	db := setupOauthUserUniquenessTest(t)
	canonical := &model.User{Username: "canonical", Nickname: "李伟铭", Email: "lwm@sweetnight.com", GroupId: 1}
	if err := db.Create(canonical).Error; err != nil {
		t.Fatal(err)
	}

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId:   "ou_new",
		Name:     "李伟铭",
		Username: "李伟铭",
		Email:    "LWM@SWEETNIGHT.COM",
	}, "feishu")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id != canonical.Id {
		t.Fatalf("resolved user = %#v, want canonical user id %d", user, canonical.Id)
	}
	var users int64
	if err := db.Model(&model.User{}).Count(&users).Error; err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("users = %d, want 1", users)
	}
}

func TestRegisterByOauthDoesNotOverwriteLocalProfileForOtherProviders(t *testing.T) {
	db := setupOauthUserUniquenessTest(t)
	autoRegister := true
	if err := db.Create(&model.Oauth{Op: "google", OauthType: model.OauthTypeGoogle, AutoRegister: &autoRegister}).Error; err != nil {
		t.Fatal(err)
	}
	canonical := &model.User{Username: "local-user", Nickname: "本地姓名", Email: "local@sweetnight.com", Avatar: "local-avatar", GroupId: 1}
	if err := db.Create(canonical).Error; err != nil {
		t.Fatal(err)
	}

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId:   "google-id",
		Name:     "OAuth Name",
		Username: "oauth-user",
		Email:    "LOCAL@SWEETNIGHT.COM",
		Picture:  "oauth-avatar",
	}, "google")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id != canonical.Id {
		t.Fatalf("resolved user = %#v, want canonical user id %d", user, canonical.Id)
	}
	if err := db.First(canonical, canonical.Id).Error; err != nil {
		t.Fatal(err)
	}
	if canonical.Nickname != "本地姓名" || canonical.Avatar != "local-avatar" {
		t.Fatalf("local profile was overwritten: %#v", canonical)
	}
}

func TestRegisterByOauthKeepsExistingBindingForOtherProviders(t *testing.T) {
	db := setupOauthUserUniquenessTest(t)
	autoRegister := true
	if err := db.Create(&model.Oauth{Op: "google", OauthType: model.OauthTypeGoogle, AutoRegister: &autoRegister}).Error; err != nil {
		t.Fatal(err)
	}
	bound := &model.User{Username: "bound-user", Email: "bound@sweetnight.com", GroupId: 1}
	other := &model.User{Username: "other-user", Email: "other@sweetnight.com", GroupId: 1}
	if err := db.Create(bound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	binding := &model.UserThird{UserId: bound.Id, OauthUser: model.OauthUser{OpenId: "google-bound"}, OauthType: model.OauthTypeGoogle, Op: "google"}
	if err := db.Create(binding).Error; err != nil {
		t.Fatal(err)
	}

	err, user := AllService.UserService.RegisterByOauth(&model.OauthUser{
		OpenId: "google-bound",
		Email:  "other@sweetnight.com",
	}, "google")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.Id != bound.Id {
		t.Fatalf("resolved user = %#v, want existing bound user id %d", user, bound.Id)
	}
	if err := db.First(binding, binding.Id).Error; err != nil {
		t.Fatal(err)
	}
	if binding.UserId != bound.Id {
		t.Fatalf("binding user id = %d, want %d", binding.UserId, bound.Id)
	}
}
