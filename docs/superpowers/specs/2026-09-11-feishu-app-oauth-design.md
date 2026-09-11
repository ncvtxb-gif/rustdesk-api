# Feishu Custom App OAuth Design

## Goal

Replace the enterprise Windows login provider from AnyCross OIDC to a Feishu Open Platform self-built app OAuth authorization-code flow. Preserve the RustDesk client-facing login contract so the deployed Windows client does not require an authentication-flow change.

## Scope

- Add a first-class `feishu` OAuth provider type to rustdesk-api.
- Keep `/api/login-options`, `/api/oidc/auth`, `/api/oidc/callback`, and `/api/oidc/auth-query` compatible with the current client.
- Build the browser authorization URL using the Feishu self-built app.
- Exchange the callback code for a Feishu user access token on the API server.
- Fetch the current Feishu user's profile and map it into the existing `model.OauthUser` contract.
- Reuse the current automatic registration, account binding, managed device identity, RustDesk ID, access-token, and callback result paths.
- Extend the existing OAuth administration API and page to accept the `feishu` provider type using the existing client ID, client secret, scopes, auto-registration, and PKCE fields.
- Keep the existing AnyCross OIDC provider record during rollout as a rollback option.

Out of scope:

- Changing the Windows client's browser-launch or callback polling implementation.
- Deleting the AnyCross OIDC configuration automatically.
- Changing RustDesk ID allocation, device binding, or unattended-control behavior.
- Storing Feishu App ID or App Secret in source control.

## Provider Configuration

The OAuth provider record uses:

- `oauth_type`: `feishu`
- `op`: a new unique provider key, recommended `company-feishu-app`
- `client_id`: Feishu self-built app App ID
- `client_secret`: Feishu self-built app App Secret
- `scopes`: only the scopes required by Feishu user authentication and user profile retrieval
- `auto_register`: enabled for the enterprise login flow
- `pkce_enable`: disabled unless the selected Feishu authorization endpoint is verified to support the existing server-side PKCE exchange

The production `device_identity.feishu_oidc_op` value changes to the new `op` only after end-to-end verification. Despite the legacy configuration field and route names containing `oidc`, they remain unchanged for client compatibility.

## Authentication Flow

1. The Windows client requests login options and receives only the configured enterprise provider.
2. The client posts its existing device payload to `/api/oidc/auth`.
3. The API validates the enterprise client and machine UUID, creates the existing one-time state cache, and returns a Feishu authorization URL.
4. The system browser authenticates the user with the Feishu self-built app and returns to `/api/oidc/callback` with `code` and `state`.
5. The API validates state through the existing cache, exchanges the code server-side, obtains a user access token, and retrieves the current Feishu user profile.
6. The adapter maps `open_id`, display name, email, and avatar into `model.OauthUser`. `open_id` is the stable binding key for this single self-built application.
7. Existing registration or binding logic resolves the RustDesk API user and stores the OAuth association under the new provider `op`.
8. The existing `/api/oidc/auth-query` response returns the RustDesk access token, user display data, and managed device identity to the client.

## Component Boundaries

### Provider model

`model/oauth.go` owns validation of the new `feishu` type and the conversion of a Feishu user-info payload to `model.OauthUser`.

### Feishu OAuth adapter

The service layer owns endpoint definitions, authorization URL construction, code exchange, user-info retrieval, HTTP status validation, Feishu business-error validation, and safe error mapping. Provider-specific response bodies and access tokens must never be logged.

### Existing OAuth orchestrator

`service.OauthService` dispatches `BeginAuth` and `Callback` by provider type. Generic OIDC behavior remains unchanged. The Feishu adapter returns the same `model.OauthUser` consumed by the current registration and binding code.

### Client-facing controller

The API controller and response schema remain compatible. Existing enterprise provider allow-list checks continue to use the configured provider `op`, independent of provider type.

## Error Handling and Security

- Reject missing or invalid provider configuration before generating an authorization URL.
- Preserve state expiration and one-time consumption semantics.
- Require a non-empty callback authorization code.
- Apply the configured outbound HTTP client and timeout to Feishu calls.
- Reject non-2xx responses and Feishu business responses that indicate failure.
- Reject profiles without `open_id` or without a usable display name/username identity.
- Do not expose upstream tokens, App Secret, raw response bodies, or personally sensitive data in logs or client error messages.
- Convert upstream failures into stable internal errors such as token retrieval or user-info retrieval failure.
- Keep the previous provider record unchanged until the new flow passes production verification.

## Testing

- Model tests: provider validation and Feishu profile mapping, including missing optional email.
- Authorization tests: generated URL contains the correct client ID, redirect URI, state, and required scopes.
- Callback tests with a local HTTP test server: successful token and user-info flow, invalid code/token response, non-2xx response, malformed JSON, Feishu business error, and missing `open_id`.
- Regression tests: existing OIDC forced-login option, login-options output, provider allow-list, redirect URL, automatic registration path, and managed identity response remain unchanged.
- Deployment verification: create the new provider without deleting the old one, configure the Feishu redirect URL as `https://rustdesk-api.sweetnight.com.cn/api/oidc/callback`, switch the enterprise provider key, complete a fresh browser login, and verify returned display name and stable device identity.

## Rollout and Rollback

1. Deploy API code that supports both `oidc` and `feishu`.
2. Add a separate `company-feishu-app` provider using server-side secrets.
3. Configure the Feishu self-built app redirect allow-list.
4. Test the new provider without removing the AnyCross record.
5. Switch `device_identity.feishu_oidc_op` to `company-feishu-app` and restart the API.
6. Verify client login, user name, device binding, and token renewal.

Rollback changes only `device_identity.feishu_oidc_op` back to the previous AnyCross provider and restarts the API. No client rollback is required.
