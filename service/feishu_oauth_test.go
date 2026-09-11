package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestBuildFeishuAuthorizationURL(t *testing.T) {
	info := &model.Oauth{ClientId: "cli_test", Scopes: "contact:user.base:readonly"}

	got, err := buildFeishuAuthorizationURL(info, "state-123", "https://api.example.com/api/oidc/callback")
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if gotEndpoint := u.Scheme + "://" + u.Host + u.Path; gotEndpoint != "https://accounts.feishu.cn/open-apis/authen/v1/authorize" {
		t.Fatalf("authorization endpoint = %q, want Feishu authorize endpoint", gotEndpoint)
	}
	q := u.Query()
	if q.Get("app_id") != "cli_test" || q.Get("state") != "state-123" || q.Get("redirect_uri") != "https://api.example.com/api/oidc/callback" {
		t.Fatalf("unexpected query: %v", q)
	}
	if q.Get("scope") != "contact:user.base:readonly" {
		t.Fatalf("scope = %q, want configured Feishu scope", q.Get("scope"))
	}
}

func TestBuildFeishuAuthorizationURLRejectsMissingClientID(t *testing.T) {
	_, err := buildFeishuAuthorizationURL(&model.Oauth{}, "state", "https://api.example.com/api/oidc/callback")
	if err == nil {
		t.Fatal("expected missing client id error")
	}
}

func TestBuildFeishuAuthorizationURLScopesAreFeishuSpecific(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes string
		want   string
	}{
		{
			name:   "trims comma separated configured scopes",
			scopes: " contact:user.base:readonly , contact:contact.base:readonly ",
			want:   "contact:user.base:readonly contact:contact.base:readonly",
		},
		{
			name: "does not inherit OIDC defaults when scopes are empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildFeishuAuthorizationURL(
				&model.Oauth{ClientId: "cli_test", Scopes: tc.scopes},
				"state-123",
				"https://api.example.com/api/oidc/callback",
			)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			if scope := u.Query().Get("scope"); scope != tc.want {
				t.Fatalf("scope = %q, want %q", scope, tc.want)
			}
		})
	}
}

func TestFeishuCallbackReturnsMappedUser(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":0,"msg":"success","data":{"open_id":"ou_123","name":"钟俊歌","email":"zhongjunge@sweetnight.com","avatar_url":"https://example.invalid/avatar.png"}}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{
		ClientId:     "cli_test",
		ClientSecret: "secret_test",
	}, "code-123")
	if err != nil {
		t.Fatal(err)
	}
	if user.OpenId != "ou_123" || user.Name != "钟俊歌" || user.Username != "zhongjunge@sweetnight.com" || user.Email != "zhongjunge@sweetnight.com" || user.Picture != "https://example.invalid/avatar.png" {
		t.Fatalf("unexpected mapped user: %#v", user)
	}
}

func TestFeishuCallbackUsesBoundedHTTPClientWithoutProxy(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":0,"msg":"success","data":{"open_id":"ou_123","name":"钟俊歌"}}`,
	})

	previousConfig := Config
	Config = &config.Config{}
	t.Cleanup(func() {
		Config = previousConfig
	})

	previousFactory := newFeishuClient
	var timeout time.Duration
	newFeishuClient = func(appID, appSecret string, httpClient larkcore.HttpClient, _ string) *lark.Client {
		client, ok := httpClient.(*http.Client)
		if !ok {
			t.Fatalf("Feishu HTTP client type = %T, want *http.Client", httpClient)
		}
		timeout = client.Timeout
		return lark.NewClient(
			appID,
			appSecret,
			lark.WithHttpClient(httpClient),
			lark.WithOpenBaseUrl(server.URL),
			lark.WithOAuthBaseUrl(server.URL),
		)
	}
	t.Cleanup(func() {
		newFeishuClient = previousFactory
	})

	err, _ := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err != nil {
		t.Fatal(err)
	}
	if timeout <= 0 {
		t.Fatalf("Feishu HTTP client timeout = %s, want a bounded timeout", timeout)
	}
}

func TestFeishuCallbackFallsBackToEnterpriseEmail(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":0,"msg":"success","data":{"open_id":"ou_123","name":"钟俊歌","enterprise_email":"zhongjunge@sweetnight.com"}}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "zhongjunge@sweetnight.com" || user.Username != "zhongjunge@sweetnight.com" {
		t.Fatalf("enterprise email fallback = %#v, want mapped enterprise email", user)
	}
}

func TestFeishuCallbackRejectsTokenBusinessFailure(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody: `{"code":99991663,"msg":"invalid authorization code"}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err == nil || err.Error() != "GetOauthTokenError" || user != nil {
		t.Fatalf("callback = (%v, %#v), want (GetOauthTokenError, nil)", err, user)
	}
}

func TestFeishuCallbackRejectsUserInfoBusinessFailure(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":99991668,"msg":"invalid user access token"}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err == nil || err.Error() != "GetOauthUserInfoError" || user != nil {
		t.Fatalf("callback = (%v, %#v), want (GetOauthUserInfoError, nil)", err, user)
	}
}

func TestFeishuCallbackRejectsMalformedUserInfo(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err == nil || err.Error() != "DecodeOauthUserInfoError" || user != nil {
		t.Fatalf("callback = (%v, %#v), want (DecodeOauthUserInfoError, nil)", err, user)
	}
}

func TestFeishuCallbackRejectsMissingOpenID(t *testing.T) {
	server := newFakeFeishuServer(t, feishuFakeResponses{
		tokenBody:    `{"code":0,"msg":"success","data":{"access_token":"u-test-token"}}`,
		userInfoBody: `{"code":0,"msg":"success","data":{"name":"钟俊歌"}}`,
	})
	useFakeFeishuClient(t, server.URL)

	err, user := (&OauthService{}).feishuCallback(&model.Oauth{ClientId: "cli_test", ClientSecret: "secret_test"}, "code-123")
	if err == nil || err.Error() != "DecodeOauthUserInfoError" || user != nil {
		t.Fatalf("callback = (%v, %#v), want (DecodeOauthUserInfoError, nil)", err, user)
	}
}

type feishuFakeResponses struct {
	tokenBody    string
	userInfoBody string
}

func newFakeFeishuServer(t *testing.T, responses feishuFakeResponses) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/app_access_token/internal":
			if r.Method != http.MethodPost {
				t.Errorf("app token method = %s, want POST", r.Method)
			}
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","app_access_token":"a-test-token","expire":7200}`)
		case "/open-apis/authen/v1/access_token":
			if r.Method != http.MethodPost {
				t.Errorf("access token method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer a-test-token" {
				t.Errorf("access token authorization = %q, want app token", got)
			}
			var body struct {
				GrantType string `json:"grant_type"`
				Code      string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode access token request: %v", err)
			} else if body.GrantType != "authorization_code" || body.Code != "code-123" {
				t.Errorf("access token request = %#v, want authorization code grant", body)
			}
			_, _ = fmt.Fprint(w, responses.tokenBody)
		case "/open-apis/authen/v1/user_info":
			if r.Method != http.MethodGet {
				t.Errorf("user-info method = %s, want GET", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer u-test-token" {
				t.Errorf("user-info authorization = %q, want user access token", got)
			}
			_, _ = fmt.Fprint(w, responses.userInfoBody)
		default:
			t.Errorf("unexpected Feishu SDK request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func useFakeFeishuClient(t *testing.T, baseURL string) {
	t.Helper()
	previousConfig := Config
	Config = &config.Config{}
	t.Cleanup(func() {
		Config = previousConfig
	})

	previousFactory := newFeishuClient
	newFeishuClient = func(appID, appSecret string, httpClient larkcore.HttpClient, _ string) *lark.Client {
		return lark.NewClient(
			appID,
			appSecret,
			lark.WithHttpClient(httpClient),
			lark.WithOpenBaseUrl(baseURL),
			lark.WithOAuthBaseUrl(baseURL),
		)
	}
	t.Cleanup(func() {
		newFeishuClient = previousFactory
	})
}
