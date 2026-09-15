# Sweetnight-PC Enterprise Address Book Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically maintain one administrator-only `Sweetnight-PC` address book for all managed Windows devices while blocking ordinary users from both that address book and the API admin portal.

**Architecture:** Keep the global collection under a technical administrator owner and upsert every managed device into it at successful desktop identity login. Enforce authorization in the Go service/controller layer, then mirror that authorization in the Vue admin UI and Flutter Windows client so hidden controls cannot be bypassed by direct API calls.

**Tech Stack:** Go, Gin, GORM, SQLite/MySQL-compatible constraints, Vue 3, Element Plus, Vite, Flutter/Dart.

**Spec:** `docs/superpowers/specs/2026-09-15-company-address-book-access.md`

## Global Constraints

- The collection name is exactly `Sweetnight-PC`.
- `AddressBook.Username` is the last successful Feishu user's `nickname`, falling back to `User.Username` only when the nickname is empty.
- Ordinary users have no read or write access to the company collection and cannot log in to the API admin portal.
- Ordinary users can still use Feishu login in the Windows desktop client.
- Alias, hash, and Web Client controls are absent from the company address-book UI.
- Preserve unrelated frontend styling and production static customizations.

---

### Task 1: Enforce administrator-only company address-book access

**Files:**
- Modify: `service/company_address_book_test.go`
- Modify: `service/addressBook.go`

**Interfaces:**
- Consumes: `UserService.IsAdmin(*model.User) bool`, `CompanyAddressBookName`.
- Produces: `AddressBookService.UserMaxRule(...)` returning `ShareAddressBookRuleRuleNone` for ordinary users and full control for administrators.

- [ ] Change the existing normal-user tests to assert no collection discovery, no visible devices, and no read/write privilege.
- [ ] Run `go test ./service -run 'TestCompany(Collection|AddressBook)|TestAdministrator' -count=1` and verify the new assertions fail because ordinary users currently receive read access.
- [ ] Remove ordinary-user rule creation from `EnsureCompanyDevice`, make company collection discovery administrator-only, and return no permission for ordinary users.
- [ ] Re-run the focused tests and verify they pass.
- [ ] Run `go test ./service ./http/... -count=1`.
- [ ] Commit backend authorization changes.

### Task 2: Reject ordinary users from API admin OAuth sessions

**Files:**
- Modify: `http/controller/admin/login.go`
- Create or modify: `http/controller/admin/login_test.go`

**Interfaces:**
- Consumes: `UserService.IsAdmin(*model.User) bool` and the OAuth polling result.
- Produces: `responseLoginSuccess` that returns 403 for non-admin users and emits no usable admin login payload.

- [ ] Add a controller-level failing test where OAuth resolves to a normal user and assert HTTP 403 with no token payload.
- [ ] Run the focused Go test and verify it fails because the current handler returns success.
- [ ] Add the administrator check immediately before the admin login response; invalidate the newly created token when practical within the existing token service boundary.
- [ ] Add the administrator success case and run both tests.
- [ ] Run `go test ./http/controller/admin ./service -count=1`.
- [ ] Commit the admin-login restriction.

### Task 3: Backfill existing devices safely

**Files:**
- Modify: `cmd/apimain.go` or the existing migration/maintenance entry point selected from current project conventions.
- Modify: `service/company_address_book_test.go`

**Interfaces:**
- Consumes: peers with a valid bound `user_id` and the unique `Sweetnight-PC` collection.
- Produces: an idempotent `BackfillCompanyAddressBook() (createdOrUpdated int, err error)` operation.

- [ ] Add failing tests for multiple existing peers, skipped unbound peers, nickname fallback, and idempotent reruns.
- [ ] Run the focused tests and verify failure because the backfill operation does not exist.
- [ ] Implement the smallest service operation that iterates bound peers and calls the same upsert rules as managed login.
- [ ] Re-run focused and full service tests.
- [ ] Expose it through the existing one-shot maintenance mechanism without running it automatically during every process start.
- [ ] Commit the backfill operation.

### Task 4: Remove sensitive/unused company address-book controls from admin UI

**Files:**
- Modify: `D:/codex/rustdesk-api-web/src/views/address_book/index.vue`
- Modify: `D:/codex/rustdesk-api-web/src/views/address_book/index.js`
- Modify: `D:/codex/rustdesk-api-web/src/views/my/address_book/index.vue`
- Modify: `D:/codex/rustdesk-api-web/src/views/my/address_book/indexv2.vue`
- Create: `D:/codex/rustdesk-api-web/test/company-address-book-ui.test.mjs`
- Modify: `D:/codex/rustdesk-api-web/package.json`

**Interfaces:**
- Consumes: collection name and address-book response fields.
- Produces: an administrator view without alias/hash/Web Client controls and with the device owner name sourced from `row.username`.

- [ ] Add a Node static/component-source test that asserts the company view excludes alias, hash, Web Client, and share controls.
- [ ] Run the test and verify it fails against the current templates.
- [ ] Introduce a focused `isCompanyCollection` rendering condition or dedicated company-table branch without changing other address-book behavior.
- [ ] Remove unused imports/state only where the company branch makes them unreachable.
- [ ] Run the new test, existing `npm run test:oauth`, and `npm run build`.
- [ ] Commit the admin frontend change without replacing unrelated style files.

### Task 5: Hide the Windows client address book for ordinary users

**Files:**
- Modify: `D:/codex/rustdesk-client/flutter/lib/common/widgets/peer_tab_page.dart`
- Modify as needed: `D:/codex/rustdesk-client/flutter/lib/desktop/pages/desktop_tab_page.dart`
- Create or modify: `D:/codex/rustdesk-client/flutter/test/enterprise_address_book_visibility_test.dart`

**Interfaces:**
- Consumes: `gFFI.userModel.isAdmin` and the enterprise Windows build predicate.
- Produces: `shouldShowEnterpriseAddressBook({required bool enterpriseWindows, required bool isAdmin})` used by tab construction.

- [ ] Add a failing Dart unit test covering enterprise ordinary user hidden, enterprise administrator shown, and non-enterprise behavior unchanged.
- [ ] Run the focused Flutter test and verify the enterprise ordinary-user case fails.
- [ ] Add the pure visibility helper and use it when constructing desktop tabs.
- [ ] Ensure login/logout/admin-state changes rebuild the visible tab set and cannot leave a stale selected address-book tab.
- [ ] Run the focused test and existing enterprise widget/model tests.
- [ ] Commit the client change.

### Task 6: Cross-repository verification and deployment preparation

**Files:**
- Modify only if required by failing verification: workflow/build metadata in the owning repository.

**Interfaces:**
- Consumes: passing backend, frontend, and client commits.
- Produces: reproducible build artifacts and a deployment checklist with rollback paths.

- [ ] Run backend full tests and build.
- [ ] Run frontend tests and production build; record dist checksums.
- [ ] Run client focused tests and the configured Windows CI/build validation.
- [ ] Review diffs for unrelated UI/style regressions and credential exposure.
- [ ] Back up server binary/config/static assets before any deployment.
- [ ] Deploy backend and frontend only after artifact verification, then verify service health and administrator/ordinary-user behavior.
- [ ] Trigger the Windows client build and verify the produced artifact provenance before release.

