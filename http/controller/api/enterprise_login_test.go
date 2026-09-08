package api

import (
	"encoding/json"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	response "github.com/lejianwen/rustdesk-api/v2/http/response/api"
)

func TestEnterpriseWindowsRequestRequiresConfiguredClientAndUUID(t *testing.T) {
	cfg := &config.DeviceIdentity{Enabled: true, EnterpriseClientType: "enterprise-windows"}
	tests := []struct {
		name string
		req  request.OidcAuthRequest
		want bool
	}{
		{"enterprise windows", request.OidcAuthRequest{Uuid: "machine-a", DeviceInfo: request.DeviceInfoInLogin{Os: "Windows", Type: "enterprise-windows"}}, true},
		{"empty uuid", request.OidcAuthRequest{DeviceInfo: request.DeviceInfoInLogin{Os: "Windows", Type: "enterprise-windows"}}, false},
		{"ordinary windows", request.OidcAuthRequest{Uuid: "machine-a", DeviceInfo: request.DeviceInfoInLogin{Os: "Windows", Type: "app"}}, false},
		{"mac", request.OidcAuthRequest{Uuid: "machine-a", DeviceInfo: request.DeviceInfoInLogin{Os: "Mac OS", Type: "enterprise-windows"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEnterpriseWindowsRequest(cfg, &tt.req); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnterpriseLoginOptionsExposeOnlyFeishu(t *testing.T) {
	cfg := &config.DeviceIdentity{Enabled: true, FeishuOidcOp: "company-feishu"}
	got := buildLoginOptions([]string{"github", "company-feishu", "google"}, true, cfg)
	if len(got) != 2 || got[1] != "oidc/company-feishu" {
		t.Fatalf("unexpected enterprise login options: %#v", got)
	}
}

func TestEnterpriseModeDisablesPasswordLogin(t *testing.T) {
	if !passwordLoginDisabled(false, &config.DeviceIdentity{Enabled: true}) {
		t.Fatal("password login remained enabled in enterprise mode")
	}
}

func TestEnterpriseModeRejectsNonFeishuOIDCProvider(t *testing.T) {
	cfg := &config.DeviceIdentity{Enabled: true, FeishuOidcOp: "company-feishu"}
	if oidcProviderAllowed(cfg, "github") {
		t.Fatal("non-Feishu provider remained callable in enterprise mode")
	}
	if !oidcProviderAllowed(cfg, "company-feishu") {
		t.Fatal("configured Feishu provider was rejected")
	}
}

func TestLoginResponseOmitsDeviceForLegacyClients(t *testing.T) {
	b, err := json.Marshal(response.LoginRes{Type: "access_token", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"type":"access_token","access_token":"token","user":{"name":"","email":"","note":"","is_admin":null,"status":0,"info":null}}` {
		t.Fatalf("legacy response contract changed: %s", b)
	}
}

func TestEnterpriseLoginResponseIncludesManagedIdentity(t *testing.T) {
	b, err := json.Marshal(response.LoginRes{Type: "access_token", AccessToken: "token", Device: &response.DeviceIdentityPayload{
		RustdeskId: "123456789", PasswordVersion: 1, Status: "active",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatal(err)
	}
	device, ok := payload["device"].(map[string]interface{})
	if !ok || device["rustdesk_id"] != "123456789" || device["password_version"] != float64(1) || device["status"] != "active" {
		t.Fatalf("unexpected enterprise device payload: %s", b)
	}
	if _, exists := device["permanent_password"]; exists {
		t.Fatalf("OIDC login leaked permanent password: %s", b)
	}
}
