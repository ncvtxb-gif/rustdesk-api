# Managed Windows device bootstrap

The Feishu OIDC success response may include `device.rustdesk_id`, `device.password_version`, and `device.status`. It never includes the permanent password.

The Windows service retrieves the secret through `POST /api/managed-device/bootstrap` using the issued bearer token:

```json
{"machine_uuid":"<RustDesk base64 machine UID>","platform":"windows"}
```

The machine UUID is RustDesk's existing case-sensitive Base64-encoded machine UID. It is trimmed at the API boundary but is otherwise opaque and must not be parsed or rewritten as an RFC UUID.

A successful response contains `rustdesk_id`, `permanent_password`, `password_version`, `status`, `machine_uuid`, and `session_expires_at` (Unix seconds). The API returns it only when the token user, token machine UUID, token device ID, and active device identity all match.

After applying the secret locally, the service uploads `SHA256(permanent_password + target_salt)` as standard Base64 with `PUT /api/managed-device/auth-hash`. Success is HTTP 204.
