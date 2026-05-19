# Changelog

All notable changes to Rute Bayar will be documented in this file.

## v0.1.1 - 2026-05-19

### Changed

- Standardized CLI command usage and installation snippets to `rutebayar` across project documentation and quickstart paths after the binary rename.
- Refreshed production-install examples to reflect the current command name and release artifact naming.

### Verified

- `go test ./...` passes on `main` after merge of the command rename/docs alignment.

## v0.1.0 - 2026-05-17

First stable release of Rute Bayar, focused on a production-shaped CLI and daemon foundation for Indonesian payment gateway routing.

### Added

- Stable CLI flows for provider onboarding, provider auth testing, payment creation, payment status checks, refund requests, reconciliation, webhook serving, webhook replay, and forwarding diagnostics.
- Xendit Payment Sessions support for `pay create`, `pay status`, async refund requests, callback-token verification, payment webhook reconciliation, and final refund webhook reconciliation.
- Midtrans Core API support for bank transfer, QRIS, and card-oriented create/status/refund flows where supported by the provider sandbox.
- Pass-through webhook forwarding with persisted targets, event filters, retry settings, and forwarding attempt diagnostics.
- SQLite persistence for provider accounts, payment intents, payment attempts, status checks, refunds, webhook events, forwarding targets, and forwarding attempts.
- Release automation for Linux, macOS, and Windows binaries with SHA-256 checksums.

### Changed

- Xendit refund webhook reconciliation is now atomic for refund row and parent payment intent updates.
- Xendit refund webhook lookup now uses exact JSON field matching instead of broad JSON `LIKE` scans.
- Xendit `pay create --notification-url` now fails explicitly because Payment Sessions do not support per-payment webhook override.
- README and release-readiness documentation now reflect stable-candidate evidence instead of alpha-only status.
- GitHub Actions dependencies and `modernc.org/sqlite` were updated through Dependabot PRs.

### Verified

- `go test ./...` and `go vet ./...` pass locally and in CI.
- Real Midtrans sandbox webhook callback reached the daemon through Cloudflare Tunnel and reconciled local status.
- Real Xendit sandbox Payment Sessions webhook callback reached the daemon through Cloudflare Tunnel and reconciled local status.
- Real Xendit sandbox refund E2E completed from refund request through final `refund.succeeded` webhook, with `refunds.status=refunded` and payment intent status `refunded`.
- Release workflow successfully builds the same platform matrix used by prior alpha releases.

### Known Limitations

- Midtrans refund success remains dependent on provider-side sandbox balance and settlement eligibility; the tested sandbox account returned insufficient balance for a settled QRIS refund attempt.
- Xendit Payment Sessions webhook URL must be configured in the Xendit Dashboard; per-payment webhook override is not supported by Xendit Payment Sessions.
- Flip Business is planned as the next provider family and is not included in this release.

## v0.1.0-alpha.3 - 2026-05-08

Third alpha release focused on sandbox E2E coverage, Midtrans refundable methods, and webhook forwarding validation.

### Added

- Midtrans Core API credit card create flow with `--card-token`.
- Midtrans Core API dynamic QRIS create flow.
- Midtrans 3DS helper page for sandbox browser authentication checks.
- E2E runner support for Midtrans card and QRIS methods.
- Additional sandbox E2E documentation for Midtrans card, QRIS, webhook tunnel, and refund checks.

### Changed

- Xendit Payment Session creation now keeps `customer.reference_id` populated for valid sandbox payloads.
- Provider status inquiry and reconcile now preserve local refunded/partial-refunded status instead of downgrading from provider payment-session status.
- Midtrans refund adapter now treats non-2xx business `status_code` responses as failed refund attempts.

### Verified

- Xendit sandbox create, payment, status, webhook, forwarding, and refund.
- Midtrans sandbox bank transfer create/status.
- Midtrans sandbox card create, 3DS authentication, webhook, and forwarding.
- Midtrans sandbox QRIS create, payment simulator settlement, webhook, and forwarding.

### Known Limitations

- Midtrans refund success could not be completed on the tested sandbox account because the provider returned `414` merchant insufficient funds despite a settled QRIS transaction.
- Midtrans card transactions can remain in `capture` until provider settlement; refund must wait for `settlement`.
- This release is still alpha and should be used for integration testing rather than production workloads.

## v0.1.0-alpha.2 - 2026-05-07

Second alpha release focused on release automation and operational hardening.

### Added

- GitHub Actions CI for formatting, vet, tests, and build matrix.
- Tag-driven GitHub Release automation for Linux, macOS, and Windows binaries.
- SHA-256 checksum generation for release artifacts.
- Forwarding attempt diagnostics:
  - `webhook forward attempts list`
  - `webhook forward attempts show <attempt-id>`
  - `webhook forward attempts retry <attempt-id>`
- Local smoke script for webhook daemon and forwarding diagnostics.
- End-to-end smoke checklist documentation.
- Provider status mapping helper and status mapping documentation.

### Changed

- Standardized forwarding diagnostic timestamps with RFC3339 formatting.
- Improved forwarding retry error handling when the service is not initialized.

## v0.1.0-alpha.1 - 2026-05-07

First alpha release for early testing and feedback.

### Added

- CLI onboarding for Midtrans and Xendit provider credentials.
- SQLite-backed provider account, payment intent, payment attempt, status check, refund, webhook event, and forwarding persistence.
- `pay create` for Xendit Payment Sessions.
- `pay create` for Midtrans Core API bank transfer flow.
- `pay status` for supported Midtrans and Xendit payment references.
- `pay refund` for supported Xendit and Midtrans refund flows.
- `reconcile` command for payment state follow-up.
- Webhook daemon with provider-specific Midtrans and Xendit routes.
- Midtrans webhook signature verification and Xendit callback token validation when configured.
- Webhook parsing and basic reconciliation into payment intent status.
- Webhook forwarding target management through CLI.
- Pass-through forwarding with configurable headers, retry policy, enabled flag, and event filters.
- `webhook replay` for replaying stored inbound webhook events through forwarding targets.
- Operations runbook and development documentation for local webhook, Cloudflare tunnel, and diagnostics.

### Changed

- Refactored payment, webhook, forwarding, and provider factory flows into smaller service packages.
- Improved test coverage for provider adapters, payment status, webhook parsing, forwarding filters, replay, refund, and SQLite storage.
- Build version can now be injected with Go `ldflags`.

### Known Limitations

- This release is alpha and not recommended as a stable production release.
- Automated release artifact builds are not available in this release.
- Webhook diagnostics are useful but still minimal; richer failed-attempt listing/export remains planned.
- Retry policy is configurable, but failure classification is still simple.
- More provider methods and edge-case coverage are expected before a stable `v0.1.0`.
