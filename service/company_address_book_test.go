package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/config"
	jwtlib "github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

func setupCompanyAddressBookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Peer{},
		&model.AddressBook{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
	); err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })
	return db
}

func testBool(value bool) *bool { return &value }

func TestEnsureCompanyDeviceCreatesPersonalEntryAndReadRule(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	user := &model.User{Username: "feishu-user", Nickname: "Feishu Display Name", IsAdmin: testBool(false)}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	if err := db.Create(collection).Error; err != nil {
		t.Fatal(err)
	}

	if err := (&AddressBookService{}).EnsureCompanyDevice(user, "123456789"); err != nil {
		t.Fatal(err)
	}

	var entry model.AddressBook
	if err := db.Where("collection_id = ? AND id = ?", collection.Id, "123456789").First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.UserId != owner.Id || entry.Username != "Feishu Display Name" {
		t.Fatalf("unexpected company entry: %+v", entry)
	}
	var rule model.AddressBookCollectionRule
	if err := db.Where("collection_id = ? AND to_id = ?", collection.Id, user.Id).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if rule.Rule != model.ShareAddressBookRuleRuleRead || rule.Type != model.ShareAddressBookRuleTypePersonal {
		t.Fatalf("unexpected company rule: %+v", rule)
	}
}

func TestEnsureCompanyDeviceRefreshesDisplayNameAndDoesNotDuplicate(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	user := &model.User{Username: "feishu-user", Nickname: "First Name", IsAdmin: testBool(false)}
	db.Create(owner)
	db.Create(user)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)
	svc := &AddressBookService{}
	if err := svc.EnsureCompanyDevice(user, "123456789"); err != nil {
		t.Fatal(err)
	}
	user.Nickname = "Updated Name"
	if err := svc.EnsureCompanyDevice(user, "123456789"); err != nil {
		t.Fatal(err)
	}
	var entries []model.AddressBook
	if err := db.Where("collection_id = ? AND id = ?", collection.Id, "123456789").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Username != "Updated Name" {
		t.Fatalf("expected one refreshed entry, got %+v", entries)
	}
}

func TestEnsureCompanyDeviceRefreshesPeerMetadataAfterBootstrap(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	user := &model.User{Username: "feishu-user", IsAdmin: testBool(false)}
	db.Create(owner)
	db.Create(user)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)
	svc := &AddressBookService{}
	if err := svc.EnsureCompanyDevice(user, "123456789"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Peer{Id: "123456789", UserId: user.Id, Hostname: "DESKTOP-01", Os: "Windows 11"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureCompanyDevice(user, "123456789"); err != nil {
		t.Fatal(err)
	}
	var entry model.AddressBook
	if err := db.Where("collection_id = ? AND id = ?", collection.Id, "123456789").First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if entry.Hostname != "DESKTOP-01" || entry.Platform != "Windows" {
		t.Fatalf("peer metadata was not refreshed: %+v", entry)
	}
}

func TestCompanyCollectionVisibilitySeparatesNormalUsers(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	first := &model.User{Username: "first", IsAdmin: testBool(false)}
	second := &model.User{Username: "second", IsAdmin: testBool(false)}
	db.Create(owner)
	db.Create(first)
	db.Create(second)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)
	db.Create(&model.Peer{Id: "111111111", UserId: first.Id})
	db.Create(&model.Peer{Id: "222222222", UserId: second.Id})
	db.Create(&model.AddressBook{Id: "111111111", UserId: owner.Id, CollectionId: collection.Id})
	db.Create(&model.AddressBook{Id: "222222222", UserId: owner.Id, CollectionId: collection.Id})

	list := (&AddressBookService{}).ListVisibleByUserAndCollection(first, owner.Id, collection.Id, 1, 1000)
	if list.Total != 1 || len(list.AddressBooks) != 1 || list.AddressBooks[0].Id != "111111111" {
		t.Fatalf("normal user saw another user's device: %+v", list.AddressBooks)
	}
	adminList := (&AddressBookService{}).ListVisibleByUserAndCollection(owner, owner.Id, collection.Id, 1, 1000)
	if adminList.Total != 2 || len(adminList.AddressBooks) != 2 {
		t.Fatalf("administrator did not see all devices: %+v", adminList.AddressBooks)
	}
}

func TestCompanyCollectionIsReadOnlyForNormalUsers(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	user := &model.User{Username: "user", IsAdmin: testBool(false)}
	db.Create(owner)
	db.Create(user)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)
	db.Create(&model.AddressBookCollectionRule{UserId: owner.Id, CollectionId: collection.Id, ToId: user.Id, Type: model.ShareAddressBookRuleTypePersonal, Rule: model.ShareAddressBookRuleRuleFullControl})
	if got := (&AddressBookService{}).UserMaxRule(user, owner.Id, collection.Id); got != model.ShareAddressBookRuleRuleRead {
		t.Fatalf("normal user retained write access to company collection: %d", got)
	}
}

func TestAdministratorAlwaysGetsCompanyCollectionFullControl(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "owner", IsAdmin: testBool(false)}
	admin := &model.User{Username: "new-admin", IsAdmin: testBool(true)}
	db.Create(owner)
	db.Create(admin)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)

	if got := (&AddressBookService{}).UserMaxRule(admin, owner.Id, collection.Id); got != model.ShareAddressBookRuleRuleFullControl {
		t.Fatalf("administrator without a persisted rule got permission %d", got)
	}
	rules := (&AddressBookService{}).CollectionReadRules(admin)
	if len(rules) != 1 || rules[0].CollectionId != collection.Id || rules[0].Rule != model.ShareAddressBookRuleRuleFullControl {
		t.Fatalf("administrator did not discover the company collection: %+v", rules)
	}
}

func TestCompanyDeviceAndRuleCompositeKeysAreUnique(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	firstEntry := &model.AddressBook{Id: "123456789", UserId: 1, CollectionId: 2}
	if err := db.Create(firstEntry).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AddressBook{Id: "123456789", UserId: 1, CollectionId: 2}).Error; err == nil {
		t.Fatal("duplicate address-book identity was accepted")
	}
	firstRule := &model.AddressBookCollectionRule{UserId: 1, CollectionId: 2, ToId: 3, Type: model.ShareAddressBookRuleTypePersonal, Rule: model.ShareAddressBookRuleRuleRead}
	if err := db.Create(firstRule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AddressBookCollectionRule{UserId: 1, CollectionId: 2, ToId: 3, Type: model.ShareAddressBookRuleTypePersonal, Rule: model.ShareAddressBookRuleRuleRead}).Error; err == nil {
		t.Fatal("duplicate address-book rule was accepted")
	}
}

func TestShouldAddCompanyDeviceOnlyForDesktopApp(t *testing.T) {
	if !shouldAddCompanyDevice(&model.LoginLog{Client: model.LoginLogClientApp, DeviceId: "123456789"}) {
		t.Fatal("desktop device was not selected for company address book")
	}
	if shouldAddCompanyDevice(&model.LoginLog{Client: model.LoginLogClientWeb, DeviceId: "123456789"}) {
		t.Fatal("web client was selected for company address book")
	}
	if shouldAddCompanyDevice(&model.LoginLog{Client: model.LoginLogClientApp}) {
		t.Fatal("empty device ID was selected for company address book")
	}
}

func TestManagedIdentityLoginAddsAllocatedDeviceToCompanyAddressBook(t *testing.T) {
	svc, db := newDeviceIdentityTestService(t)
	if err := db.AutoMigrate(&model.User{}, &model.Peer{}, &model.AddressBook{}, &model.AddressBookCollection{}, &model.AddressBookCollectionRule{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldServices, oldConfig, oldJWT := DB, AllService, Config, Jwt
	DB = db
	Config = &config.Config{}
	Jwt = jwtlib.NewJwt("", 0)
	AllService = &Service{DeviceIdentityService: svc, AddressBookService: &AddressBookService{}}
	t.Cleanup(func() { DB, AllService, Config, Jwt = oldDB, oldServices, oldConfig, oldJWT })

	owner := &model.User{Username: "owner", IsAdmin: testBool(true)}
	user := &model.User{Username: "feishu-user", Nickname: "Feishu Display Name", IsAdmin: testBool(false)}
	db.Create(owner)
	db.Create(user)
	collection := &model.AddressBookCollection{UserId: owner.Id, Name: CompanyAddressBookName}
	db.Create(collection)

	_, identity, _, err := svc.LoginWithDeviceIdentity(user, &model.LoginLog{UserId: user.Id, Uuid: testOpaqueMachineUUID, Client: "enterprise-windows", Platform: "windows"})
	if err != nil {
		t.Fatal(err)
	}
	var entry model.AddressBook
	if err := db.Where("collection_id = ? AND id = ?", collection.Id, identity.RustdeskId).First(&entry).Error; err != nil {
		t.Fatalf("managed login did not add allocated ID to company address book: %v", err)
	}
}
