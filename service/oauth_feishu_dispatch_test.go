package service

import (
	"net/url"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

func setupOauthDispatchTest(t *testing.T, provider *model.Oauth) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Oauth{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(provider).Error; err != nil {
		t.Fatal(err)
	}

	previousDB := DB
	previousConfig := Config
	DB = db
	Config = &config.Config{Rustdesk: config.Rustdesk{ApiServer: "https://api.example.com"}}
	t.Cleanup(func() {
		DB = previousDB
		Config = previousConfig
	})
}

func TestBeginAuthDispatchesFeishu(t *testing.T) {
	autoRegister := true
	setupOauthDispatchTest(t, &model.Oauth{
		Op:           "company-feishu-app",
		OauthType:    model.OauthTypeFeishu,
		ClientId:     "cli_test",
		ClientSecret: "secret",
		AutoRegister: &autoRegister,
	})

	err, state, verifier, nonce, authURL := (&OauthService{}).BeginAuth("company-feishu-app", true)
	if err != nil {
		t.Fatal(err)
	}
	if state == "" {
		t.Fatal("state is empty")
	}
	if verifier != "" || nonce != "" {
		t.Fatalf("Feishu PKCE values = (%q, %q), want empty verifier and nonce", verifier, nonce)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("state"); got != state {
		t.Fatalf("authorization state = %q, want %q", got, state)
	}
	if got := parsed.Query().Get("prompt"); got != "" {
		t.Fatalf("Feishu prompt = %q, want empty", got)
	}
}

func TestCallbackDispatchesFeishu(t *testing.T) {
	autoRegister := true
	setupOauthDispatchTest(t, &model.Oauth{
		Op:           "company-feishu-app",
		OauthType:    model.OauthTypeFeishu,
		ClientId:     "cli_test",
		ClientSecret: "secret_test",
		AutoRegister: &autoRegister,
	})
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":0,"msg":"success","data":{"open_id":"ou_123","name":"钟俊歌","email":"zhongjunge@sweetnight.com"}}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).Callback("code-123", "", "company-feishu-app", "")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.OpenId != "ou_123" || user.Name != "钟俊歌" {
		t.Fatalf("unexpected callback user: %#v", user)
	}
}

func TestCallbackRejectsEmptyFeishuCode(t *testing.T) {
	setupOauthDispatchTest(t, &model.Oauth{
		Op:           "company-feishu-app",
		OauthType:    model.OauthTypeFeishu,
		ClientId:     "cli_test",
		ClientSecret: "secret_test",
	})

	err, user := (&OauthService{}).Callback("", "", "company-feishu-app", "")
	if err == nil || user != nil {
		t.Fatalf("Callback() = (%v, %#v), want error and nil user", err, user)
	}
}
