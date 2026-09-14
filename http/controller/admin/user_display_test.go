package admin

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestApplyUserDisplayNamesUsesNicknameWithoutChangingIdentity(t *testing.T) {
	users := &model.UserList{Users: []*model.User{
		{Username: "ou_123", Nickname: "钟俊歌"},
		{Username: "admin"},
	}}

	applyUserDisplayNames(users)

	if users.Users[0].Username != "钟俊歌" {
		t.Fatalf("display username=%q; want nickname", users.Users[0].Username)
	}
	if users.Users[1].Username != "admin" {
		t.Fatalf("fallback username=%q; want original username", users.Users[1].Username)
	}
}
