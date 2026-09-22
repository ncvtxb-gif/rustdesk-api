package service

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestPeerListDisplayFieldsSeparateAccountAndSystemUsers(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	user := &model.User{Username: "ou_internal", Nickname: "钟俊歌", IsAdmin: testBool(false)}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Peer{Id: "123456789", UserId: user.Id, Username: "Administrator", Hostname: "DESKTOP-01"}).Error; err != nil {
		t.Fatal(err)
	}

	peers := (&PeerService{}).List(1, 10, nil)
	if err := (&PeerService{}).PopulateDisplayFields(peers.Peers); err != nil {
		t.Fatal(err)
	}

	if got := peers.Peers[0].UserDisplayName; got != "钟俊歌" {
		t.Fatalf("user display name=%q; want nickname", got)
	}
	if got := peers.Peers[0].SystemUsername; got != "Administrator" {
		t.Fatalf("system username=%q; want peer username", got)
	}
}

func TestAddressBookDisplayFieldsComeFromMatchingPeer(t *testing.T) {
	db := setupCompanyAddressBookTestDB(t)
	owner := &model.User{Username: "admin", IsAdmin: testBool(true)}
	deviceUser := &model.User{Username: "ou_internal", Nickname: "钟俊歌", IsAdmin: testBool(false)}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(deviceUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Peer{Id: "123456789", UserId: deviceUser.Id, Username: "Administrator", Hostname: "DESKTOP-01"}).Error; err != nil {
		t.Fatal(err)
	}
	entry := &model.AddressBook{Id: "123456789", UserId: owner.Id, Username: "legacy-value", Hostname: "DESKTOP-01"}

	if err := (&AddressBookService{}).PopulateDisplayFields([]*model.AddressBook{entry}); err != nil {
		t.Fatal(err)
	}

	if entry.UserDisplayName != "钟俊歌" {
		t.Fatalf("user display name=%q; want peer owner nickname", entry.UserDisplayName)
	}
	if entry.SystemUsername != "Administrator" {
		t.Fatalf("system username=%q; want peer username", entry.SystemUsername)
	}
}

func TestDeviceDisplayNameStaysEmptyWhenNicknameIsEmpty(t *testing.T) {
	user := &model.User{Username: "fallback-user"}
	if got := userDisplayName(user); got != "" {
		t.Fatalf("display name=%q; want empty nickname to remain empty", got)
	}
}
