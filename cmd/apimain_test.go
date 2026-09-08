package main

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type invalidMigrationModel struct {
	Broken chan int
}

func TestMigrationFailureDoesNotRecordVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateSchemaAndRecordVersion(db, 999, &invalidMigrationModel{}); err == nil {
		t.Fatal("expected migration failure")
	}
	if db.Migrator().HasTable(&model.Version{}) {
		var count int64
		if err := db.Model(&model.Version{}).Where("version = ?", 999).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("failed migration recorded a database version")
		}
	}
}

func TestBackfillManagedUserTokensSurvivesLoginLogDeletion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserToken{}, &model.DeviceIdentity{}, &model.LoginLog{}); err != nil {
		t.Fatal(err)
	}
	managed := &model.UserToken{UserId: 7, DeviceUuid: "machine", DeviceId: "123456789", Token: "managed"}
	orphaned := &model.UserToken{UserId: 7, DeviceUuid: "orphan-machine", DeviceId: "987654321", Token: "orphaned"}
	legacy := &model.UserToken{UserId: 7, DeviceUuid: "legacy-machine", DeviceId: "legacy-id", Token: "legacy"}
	if err := db.Create(managed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(orphaned).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.DeviceIdentity{UserId: 7, MachineUuid: "machine", RustdeskId: "123456789", AllocationUuid: "allocation", CredentialCiphertext: "cipher", CredentialNonce: "nonce", Status: model.DeviceIdentityStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.LoginLog{UserId: 7, UserTokenId: orphaned.Id, Client: "enterprise-windows", Platform: "windows", Type: model.LoginLogTypeOauth, Uuid: orphaned.DeviceUuid, DeviceId: orphaned.DeviceId}).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillManagedUserTokens(db, "enterprise-windows"); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("user_token_id = ?", orphaned.Id).Delete(&model.LoginLog{}).Error; err != nil {
		t.Fatal(err)
	}
	var gotManaged, gotOrphaned, gotLegacy model.UserToken
	if err := db.First(&gotManaged, managed.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotLegacy, legacy.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotOrphaned, orphaned.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !gotManaged.Managed {
		t.Fatal("existing managed token was not backfilled")
	}
	if gotLegacy.Managed {
		t.Fatal("legacy token was incorrectly marked managed")
	}
	if !gotOrphaned.Managed {
		t.Fatal("login-log-backed managed token lost its durable marker")
	}
}
