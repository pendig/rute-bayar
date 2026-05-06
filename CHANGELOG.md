# Changelog

All notable changes to Rute Bayar will be documented in this file.

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
- GitHub Actions CI and automated release artifact builds are still pending.
- Webhook diagnostics are useful but still minimal; richer failed-attempt listing/export remains planned.
- Retry policy is configurable, but failure classification is still simple.
- More provider methods and edge-case coverage are expected before a stable `v0.1.0`.
