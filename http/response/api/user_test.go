package api

import (
	"encoding/json"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestUserPayloadSerializesNicknameAsDisplayName(t *testing.T) {
	user := &model.User{
		Username: "feishu-account",
		Nickname: "飞书姓名",
	}

	payload := (&UserPayload{}).FromUser(user)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal user payload: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal user payload: %v", err)
	}
	if got := body["display_name"]; got != "飞书姓名" {
		t.Fatalf("display_name = %#v, want %q; payload: %s", got, "飞书姓名", data)
	}
}
