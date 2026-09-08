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
