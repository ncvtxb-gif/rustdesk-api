# Feishu Custom App OAuth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add direct Feishu self-built-app OAuth login to rustdesk-api while preserving the existing RustDesk client login endpoints and keeping AnyCross OIDC available for rollback.

**Architecture:** Introduce a provider-specific Feishu adapter behind the existing `OauthService.BeginAuth` and `OauthService.Callback` orchestration. The adapter uses the official Feishu Go SDK for the typed authorization-code exchange and user-info request, then maps the result into the existing `model.OauthUser`; all registration, device identity, token response, and client polling code remains unchanged. The separately maintained admin frontend gains a `Feishu` provider choice, and the API build workflow consumes the user's frontend fork.

**Tech Stack:** Go 1.23.10, Gin, GORM, `github.com/larksuite/oapi-sdk-go/v3` v3.12.0, Vue 3, Element Plus, Node 20, Go `testing`, Node `node:test`.

**Spec:** `docs/superpowers/specs/2026-09-11-feishu-app-oauth-design.md`

## Global Constraints

- Keep `/api/login-options`, `/api/oidc/auth`, `/api/oidc/callback`, and `/api/oidc/auth-query` compatible with the current Windows client.
- Use provider type `feishu` and recommended provider key `company-feishu-app`.
- Use Feishu `open_id` as the stable binding key for this single self-built application.
- Keep App ID, App Secret, user access tokens, and raw provider responses out of source control and logs.
- Do not delete or mutate the existing AnyCross OIDC provider during code deployment.
- Do not change RustDesk ID allocation, device binding, managed credentials, or unattended-control behavior.
- Keep `device_identity.feishu_oidc_op` unchanged until the new provider passes end-to-end verification.
- Do not include the separately retained Windows account-button UI change in this plan.

---

### Task 1: Add the Feishu provider model and identity mapping

**Files:**
- Modify: `model/oauth.go`
- Create: `model/oauth_feishu_test.go`

**Interfaces:**
- Consumes: Feishu user profile fields returned by the SDK.
- Produces: `model.OauthTypeFeishu`, `model.FeishuUser`, and `(*FeishuUser).ToOauthUser() *OauthUser`.

- [ ] **Step 1: Write failing provider-validation and mapping tests**

```go
package model

import "testing"

func TestValidateOauthTypeAcceptsFeishu(t *testing.T) {
	if err := ValidateOauthType(OauthTypeFeishu); err != nil {
		t.Fatalf("ValidateOauthType(feishu): %v", err)
	}
}

func TestFeishuUserToOauthUser(t *testing.T) {
	u := (&FeishuUser{
		OpenID: "ou_123",
		Name: "钟俊歌",
		Email: "zhongjunge@sweetnight.com",
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
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./model -run 'TestValidateOauthTypeAcceptsFeishu|TestFeishuUserToOauthUser' -count=1
```

Expected: compilation fails because `OauthTypeFeishu` and `FeishuUser` do not exist.

- [ ] **Step 3: Add the minimal model implementation**

Add `OauthTypeFeishu = "feishu"`, accept it in `ValidateOauthType`, preserve a caller-supplied `Op` in `FormatOauthInfo`, and add:

```go
type FeishuUser struct {
	OpenID   string
	Name     string
	Email    string
	AvatarURL string
}

func (fu *FeishuUser) ToOauthUser() *OauthUser {
	username := strings.ToLower(strings.TrimSpace(fu.Email))
	if username == "" {
		username = fu.OpenID
	}
	return &OauthUser{
		OpenId: fu.OpenID, Name: fu.Name, Username: username,
		Email: fu.Email, VerifiedEmail: fu.Email != "", Picture: fu.AvatarURL,
	}
}
```

Also default an empty Feishu `Op` to `feishu`, matching the existing OIDC default behavior.

- [ ] **Step 4: Run the model tests and verify GREEN**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./model -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the model change**

```powershell
git add model/oauth.go model/oauth_feishu_test.go
git commit -m "feat: add Feishu OAuth provider model"
```

---

### Task 2: Implement the Feishu authorization and callback adapter

**Files:**
- Create: `service/feishu_oauth.go`
- Create: `service/feishu_oauth_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `*model.Oauth`, callback code, state, API redirect URL, configured outbound HTTP client.
- Produces: `buildFeishuAuthorizationURL(info *model.Oauth, state, redirectURL string) (string, error)` and `(*OauthService).feishuCallback(info *model.Oauth, code string) (error, *model.OauthUser)`.

- [ ] **Step 1: Add the official Feishu SDK dependency**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' get github.com/larksuite/oapi-sdk-go/v3@v3.12.0
```

Expected: `go.mod` and `go.sum` include v3.12.0; no source code changes yet.

- [ ] **Step 2: Write failing authorization URL tests**

```go
func TestBuildFeishuAuthorizationURL(t *testing.T) {
	info := &model.Oauth{ClientId: "cli_test", Scopes: "contact:user.base:readonly"}
	got, err := buildFeishuAuthorizationURL(info, "state-123", "https://api.example.com/api/oidc/callback")
	if err != nil { t.Fatal(err) }
	u, err := url.Parse(got)
	if err != nil { t.Fatal(err) }
	if u.Scheme+"://"+u.Host+u.Path != "https://accounts.feishu.cn/open-apis/authen/v1/authorize" {
		t.Fatalf("unexpected endpoint: %s", got)
	}
	q := u.Query()
	if q.Get("app_id") != "cli_test" || q.Get("state") != "state-123" ||
		q.Get("redirect_uri") != "https://api.example.com/api/oidc/callback" {
		t.Fatalf("unexpected query: %v", q)
	}
}

func TestBuildFeishuAuthorizationURLRejectsMissingClientID(t *testing.T) {
	_, err := buildFeishuAuthorizationURL(&model.Oauth{}, "state", "https://api.example.com/api/oidc/callback")
	if err == nil { t.Fatal("expected missing client id error") }
}
```

- [ ] **Step 3: Run URL tests and verify RED**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run TestBuildFeishuAuthorizationURL -count=1
```

Expected: compilation fails because `buildFeishuAuthorizationURL` does not exist.

- [ ] **Step 4: Implement URL construction without string concatenation**

```go
const feishuAuthorizeURL = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"

func buildFeishuAuthorizationURL(info *model.Oauth, state, redirectURL string) (string, error) {
	if strings.TrimSpace(info.ClientId) == "" { return "", errors.New("ConfigNotFound") }
	u, _ := url.Parse(feishuAuthorizeURL)
	q := u.Query()
	q.Set("app_id", info.ClientId)
	q.Set("redirect_uri", redirectURL)
	q.Set("state", state)
	if scopes := strings.Join((&OauthService{}).constructScopes(info.Scopes), " "); scopes != "" {
		q.Set("scope", scopes)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
```

For Feishu, an empty scope string must not inherit `OIDC_DEFAULT_SCOPES`; add a Feishu-specific scope parser that returns an empty slice for empty input and trims comma-separated values.

- [ ] **Step 5: Run URL tests and verify GREEN**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run TestBuildFeishuAuthorizationURL -count=1
```

Expected: PASS.

- [ ] **Step 6: Write failing callback tests against a local fake Feishu server**

Add a package-level SDK factory seam used only to replace endpoint bases in tests:

```go
type feishuClientFactory func(appID, appSecret string, httpClient larkcore.HttpClient, openBaseURL string) *lark.Client
```

Test success, token failure, user-info business failure, malformed response, and missing `open_id`. The success fake must serve the SDK app-token request, `/open-apis/authen/v1/access_token`, and `/open-apis/authen/v1/user_info`; assert that the user-info call receives `Authorization: Bearer u-test-token` and that the mapped result contains `open_id`, name, email, and avatar.

Representative success assertion:

```go
err, user := svc.feishuCallback(info, "code-123")
if err != nil { t.Fatal(err) }
if user.OpenId != "ou_123" || user.Name != "钟俊歌" || user.Username != "zhongjunge@sweetnight.com" {
	 t.Fatalf("unexpected user: %#v", user)
}
```

- [ ] **Step 7: Run callback tests and verify RED**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run TestFeishuCallback -count=1
```

Expected: compilation fails because `feishuCallback` and its factory seam do not exist.

- [ ] **Step 8: Implement the SDK-backed callback**

Use:

```go
client := lark.NewClient(info.ClientId, info.ClientSecret,
	lark.WithHttpClient(getHTTPClientWithProxy()),
)
tokenResp, err := client.Authen.V1.AccessToken.Create(ctx,
	larkauthen.NewCreateAccessTokenReqBuilder().
		Body(larkauthen.NewCreateAccessTokenReqBodyBuilder().
			GrantType("authorization_code").Code(code).Build()).Build())
```

Check `err`, `tokenResp.Success()`, `tokenResp.Data`, and `tokenResp.Data.AccessToken`. Fetch the profile using:

```go
userResp, err := client.Authen.V1.UserInfo.Get(ctx,
	larkcore.WithUserAccessToken(*tokenResp.Data.AccessToken))
```

Check `userResp.Success()`, `userResp.Data`, and non-empty `OpenId`. Prefer `Email`; fall back to `EnterpriseEmail`; map `Name` and `AvatarUrl`. Return stable internal errors (`GetOauthTokenError`, `GetOauthUserInfoError`, `DecodeOauthUserInfoError`) and log only error class, Feishu business code, and request ID—never the token or raw response body.

- [ ] **Step 9: Run callback and service tests and verify GREEN**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run 'TestBuildFeishuAuthorizationURL|TestFeishuCallback' -count=1
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit the adapter**

```powershell
git add go.mod go.sum service/feishu_oauth.go service/feishu_oauth_test.go
git commit -m "feat: implement Feishu app OAuth adapter"
```

---

### Task 3: Dispatch Feishu through the existing client-compatible OAuth flow

**Files:**
- Modify: `service/oauth.go`
- Create: `service/oauth_feishu_dispatch_test.go`
- Modify: `service/oauth_prompt_test.go`

**Interfaces:**
- Consumes: Task 2's `buildFeishuAuthorizationURL` and `feishuCallback`.
- Produces: unchanged public methods `BeginAuth(op string, forceReauthentication bool)` and `Callback(code, verifier, op, nonce string)` with Feishu dispatch.

- [ ] **Step 1: Write failing dispatch tests**

Use the package's test database setup to insert:

```go
&model.Oauth{
	Op: "company-feishu-app", OauthType: model.OauthTypeFeishu,
	ClientId: "cli_test", ClientSecret: "secret", AutoRegister: boolPtr(true),
}
```

Assert that `BeginAuth("company-feishu-app", true)` returns non-empty state, empty verifier and nonce, and a Feishu authorization URL whose state matches the returned state. Add a callback dispatch test using the Task 2 fake client factory. Add a regression assertion that OIDC enterprise login still adds `prompt=login` while Feishu does not add an unsupported prompt parameter.

- [ ] **Step 2: Run dispatch tests and verify RED**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run 'TestBeginAuthDispatchesFeishu|TestCallbackDispatchesFeishu|TestOauthAuthCodeOptions' -count=1
```

Expected: Feishu dispatch fails with `unsupported OAuth type` or attempts OIDC discovery.

- [ ] **Step 3: Add minimal provider dispatch**

In `BeginAuth`, load and validate the provider record before generic OAuth configuration. When `OauthType == model.OauthTypeFeishu`, generate the state exactly as today, leave verifier and nonce empty, and return `buildFeishuAuthorizationURL(oauthInfo, state, oauthRedirectURL(Config.Rustdesk.ApiServer))`.

In `Callback`, load the provider record first. When its type is Feishu, reject an empty code and return `os.feishuCallback(oauthInfo, code)`; otherwise call the existing generic `GetOauthConfig` and preserve all current GitHub, LinuxDo, Google, and OIDC branches.

- [ ] **Step 4: Run dispatch and regression tests and verify GREEN**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./service -run 'TestBeginAuthDispatchesFeishu|TestCallbackDispatchesFeishu|TestOauthAuthCodeOptions|TestOauthRedirectURL' -count=1
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./http/controller/api -run 'TestBuildLoginOptions|TestOidcProviderAllowed|TestEnterprise' -count=1
```

Expected: PASS; existing client routes and returned option strings remain `oidc/<op>`.

- [ ] **Step 5: Commit orchestration**

```powershell
git add service/oauth.go service/oauth_feishu_dispatch_test.go service/oauth_prompt_test.go
git commit -m "feat: route enterprise login through Feishu OAuth"
```

---

### Task 4: Add Feishu to the separate OAuth administration frontend

**Repositories and files:**
- Create fork if absent: `https://github.com/ncvtxb-gif/rustdesk-api-web`
- Modify in frontend fork: `src/views/oauth/index.vue`
- Create in frontend fork: `src/views/oauth/provider.mjs`
- Create in frontend fork: `test/provider.test.mjs`
- Modify in frontend fork: `package.json`
- Modify in API repo: `.github/workflows/build.yml`
- Modify in API repo: `.github/workflows/build_test.yml`

**Interfaces:**
- Consumes: backend provider type `feishu` and existing OAuth create/update payload.
- Produces: a management form that creates `company-feishu-app` without requiring an issuer and with PKCE disabled by default; API builds use the user's frontend fork.

- [ ] **Step 1: Create or clone the user's frontend fork in an isolated worktree**

Check first:

```powershell
gh repo view ncvtxb-gif/rustdesk-api-web
```

If it does not exist, create the fork without changing upstream:

```powershell
gh repo fork lejianwen/rustdesk-api-web --clone=false
```

Then clone it and create branch `feat/feishu-app-oauth-admin` using the worktree procedure.

- [ ] **Step 2: Write a failing provider-form unit test**

`test/provider.test.mjs`:

```js
import test from 'node:test'
import assert from 'node:assert/strict'
import { oauthProviderTypes, normalizeProviderForm } from '../src/views/oauth/provider.mjs'

test('offers Feishu self-built app OAuth', () => {
  assert.ok(oauthProviderTypes.some(x => x.value === 'feishu'))
})

test('normalizes Feishu provider fields', () => {
  const form = normalizeProviderForm({ oauth_type: 'feishu', op: '', issuer: 'old', pkce_enable: true })
  assert.equal(form.op, 'company-feishu-app')
  assert.equal(form.issuer, '')
  assert.equal(form.pkce_enable, false)
})
```

- [ ] **Step 3: Run the frontend test and verify RED**

```powershell
npm run test:oauth
```

Expected: command or module fails because `provider.mjs` and the script do not exist.

- [ ] **Step 4: Implement provider form behavior**

Create `provider.mjs` exporting the existing provider choices plus:

```js
{ value: 'feishu', label: '飞书自建应用' }
```

`normalizeProviderForm` must default `op` to `company-feishu-app`, clear `issuer`, and force `pkce_enable=false` for Feishu. Update `index.vue` to:

- show the IdP/op field for `oidc` and `feishu`;
- show issuer only for `oidc`;
- show scopes for `oidc` and `feishu`;
- hide and force-disable PKCE for `feishu`;
- require issuer only when the selected type is `oidc`;
- display the unchanged callback URL `/api/oidc/callback`.

Add `"test:oauth": "node --test test/provider.test.mjs"` to `package.json`.

- [ ] **Step 5: Run frontend tests and production build**

```powershell
npm install
npm run test:oauth
npm run build
```

Expected: tests PASS and Vite build exits 0.

- [ ] **Step 6: Commit and push the frontend fork**

```powershell
git add src/views/oauth/index.vue src/views/oauth/provider.mjs test/provider.test.mjs package.json package-lock.json
git commit -m "feat: configure Feishu app OAuth providers"
git push -u origin feat/feishu-app-oauth-admin
```

- [ ] **Step 7: Point API workflows at the controlled frontend source**

Change both workflow defaults from `lejianwen/rustdesk-api-web` to `ncvtxb-gif/rustdesk-api-web` and add a `WEBCLIENT_SOURCE_REF` input/default set to the merged frontend branch. Ensure checkout uses both repository and ref inputs.

- [ ] **Step 8: Validate workflow YAML and commit**

```powershell
git diff --check
rg -n "ncvtxb-gif/rustdesk-api-web|WEBCLIENT_SOURCE_REF" .github/workflows/build.yml .github/workflows/build_test.yml
git add .github/workflows/build.yml .github/workflows/build_test.yml
git commit -m "build: use managed RustDesk API admin frontend"
```

Expected: both workflows name the controlled fork and explicit ref.

---

### Task 5: Complete regression, security, and build verification

**Files:**
- Modify only if a verified failure requires a focused fix: files introduced or touched in Tasks 1–4.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: a releasable API branch and admin frontend branch with verification evidence.

- [ ] **Step 1: Run formatting and static checks**

```powershell
gofmt -w model/oauth.go model/oauth_feishu_test.go service/feishu_oauth.go service/feishu_oauth_test.go service/oauth.go service/oauth_feishu_dispatch_test.go service/oauth_prompt_test.go
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' vet ./model ./service ./http/controller/api ./http/controller/admin
git diff --check
```

Expected: exit 0 and no formatting errors.

- [ ] **Step 2: Run focused regression suites**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./model ./service ./http/controller/api ./http/controller/admin -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the full Go suite and classify known external-service failures**

```powershell
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' test ./... -count=1
```

Expected: all repository-owned tests pass. If `lib/cache/TestRedisCacheSet` is the only failure because Redis is unavailable, record it explicitly as the pre-existing external dependency; do not call the full suite green.

- [ ] **Step 4: Build the Linux deployment binary**

```powershell
$env:GOOS='linux'
$env:GOARCH='amd64'
& 'D:\codex\.toolchains\go1.23.10\go\bin\go.exe' build -trimpath -o release/rustdesk-api-linux-amd64 ./cmd/apimain.go
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
Get-FileHash -Algorithm SHA256 release/rustdesk-api-linux-amd64
```

Expected: binary exists and SHA-256 is recorded.

- [ ] **Step 5: Review secret safety**

```powershell
rg -n "cli_[A-Za-z0-9]+|app_secret|access_token" model service docs .github
```

Expected: only field names, documentation, test-only dummy values, and response schema names appear; no real credentials or user tokens are committed.

- [ ] **Step 6: Request code review and fix only verified findings**

Use `superpowers:requesting-code-review`, review the complete diff against the spec, and rerun the focused tests after every accepted correction.

---

### Task 6: Deploy safely and switch only after live Feishu verification

**Files and external configuration:**
- Server binary: `/usr/bin/rustdesk-api`
- Server config: `/var/lib/rustdesk-api/conf/config.yaml`
- OAuth provider database through the admin page or admin API.
- Feishu self-built app redirect allow-list.

**Interfaces:**
- Consumes: verified binary, Feishu App ID/App Secret supplied through the authenticated admin page, callback URL, new provider key.
- Produces: production enterprise login through the Feishu self-built app with a one-setting rollback path.

- [ ] **Step 1: Merge and push reviewed API/frontend branches**

Use `superpowers:finishing-a-development-branch`. Do not force-push. Confirm GitHub Actions succeeds before deployment.

- [ ] **Step 2: Configure the Feishu application**

In Feishu Open Platform, add this exact redirect URL:

```text
https://rustdesk-api.sweetnight.com.cn/api/oidc/callback
```

Enable only the scopes required by the login authorization and returned user profile fields.

- [ ] **Step 3: Add the new provider without touching AnyCross**

Create through the authenticated admin page:

```text
Type: feishu
IdP/op: company-feishu-app
ClientId: supplied Feishu App ID
ClientSecret: supplied Feishu App Secret
Issuer: empty
Scopes: approved Feishu user scopes
PKCE: disabled
AutoRegister: enabled
```

Verify the provider list contains both the old OIDC record and `company-feishu-app`.

- [ ] **Step 4: Back up and deploy the API binary**

Resolve server credentials from `D:\codex\rustdesk\server.txt` without printing them. On the server, copy `/usr/bin/rustdesk-api` to a timestamped backup, upload the verified binary, preserve executable mode, restart `rustdesk-api.service`, and verify `systemctl is-active rustdesk-api.service` returns `active`.

- [ ] **Step 5: Test the provider before switching enterprise clients**

Call `/api/oidc/auth` with `op=company-feishu-app` from a controlled test request, open the returned authorization URL, complete Feishu login, and poll the returned state through `/api/oidc/auth-query`. Verify:

- callback succeeds;
- response includes an access token;
- `user.display_name` is the Feishu name;
- managed device response contains the expected RustDesk ID;
- the same Feishu user maps to the same API user on a repeat test;
- no App Secret or user access token appears in service logs.

- [ ] **Step 6: Switch the enterprise provider key and restart**

Back up `/var/lib/rustdesk-api/conf/config.yaml`, set only:

```yaml
device_identity:
  feishu_oidc_op: company-feishu-app
```

Preserve all sibling keys, restart `rustdesk-api.service`, and verify `/api/login-options` returns only `oidc/company-feishu-app` for enterprise clients.

- [ ] **Step 7: Complete Windows client acceptance**

On a test device, click “使用飞书登录”, complete Feishu authorization, and verify the client returns from the browser, displays the correct name, preserves the device RustDesk ID, and reaches the normal main screen. Log out and repeat with a second authorized Feishu account to confirm the new provider behavior.

- [ ] **Step 8: Verify rollback without executing it**

Record the previous AnyCross provider key and confirm rollback requires only restoring that key in `device_identity.feishu_oidc_op` and restarting the service. Do not delete the new or old provider record during acceptance.
