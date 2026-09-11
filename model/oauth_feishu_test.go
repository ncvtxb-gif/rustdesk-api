package model

import "testing"

func TestValidateOauthTypeAcceptsFeishu(t *testing.T) {
	if err := ValidateOauthType(OauthTypeFeishu); err != nil {
		t.Fatalf("ValidateOauthType(feishu): %v", err)
	}
}

func TestFeishuUserToOauthUser(t *testing.T) {
	u := (&FeishuUser{
		OpenID:    "ou_123",
		Name:      "钟俊歌",
		Email:     "zhongjunge@sweetnight.com",
		AvatarURL: "https://example.invalid/avatar.png",
	}).ToOauthUser()
	if u.OpenId != "ou_123" || u.Name != "钟俊歌" || u.Username != "zhongjunge@sweetnight.com" {
		t.Fatalf("unexpected mapped user: %#v", u)
	}
}

func TestFeishuUserToOauthUserFallsBackWithoutEmail(t *testing.T) {
	u := (&FeishuUser{OpenID: "ou_456", Name: "李伟铭"}).ToOauthUser()
	if u.Username != "ou_456" || u.Email != "" {
		t.Fatalf("unexpected fallback user: %#v", u)
	}
}

func TestFormatOauthInfoPreservesFeishuOp(t *testing.T) {
	oa := &Oauth{OauthType: OauthTypeFeishu, Op: "custom-feishu"}
	if err := oa.FormatOauthInfo(); err != nil {
		t.Fatalf("FormatOauthInfo(feishu): %v", err)
	}
	if oa.Op != "custom-feishu" {
		t.Fatalf("expected caller-supplied op to be preserved, got %q", oa.Op)
	}
}

func TestFormatOauthInfoDefaultsEmptyFeishuOp(t *testing.T) {
	oa := &Oauth{OauthType: OauthTypeFeishu}
	if err := oa.FormatOauthInfo(); err != nil {
		t.Fatalf("FormatOauthInfo(feishu): %v", err)
	}
	if oa.Op != OauthTypeFeishu {
		t.Fatalf("expected default op %q, got %q", OauthTypeFeishu, oa.Op)
	}
}
