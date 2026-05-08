# Rute Bayar

[![CI](https://github.com/pendig/rute-bayar/actions/workflows/ci.yml/badge.svg)](https://github.com/pendig/rute-bayar/actions/workflows/ci.yml)
[![Release](https://github.com/pendig/rute-bayar/actions/workflows/release.yml/badge.svg)](https://github.com/pendig/rute-bayar/actions/workflows/release.yml)
[![GitHub Release](https://img.shields.io/github/v/release/pendig/rute-bayar?include_prereleases)](https://github.com/pendig/rute-bayar/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/pendig/rute-bayar.svg)](https://pkg.go.dev/github.com/pendig/rute-bayar)

Rute Bayar is an open source payment router for Indonesian payment gateways.

The project provides one internal interface for multiple providers, starting with **Xendit** and **Midtrans**. It is designed as a Go CLI and daemon that can create payments, receive provider webhooks, store raw JSON traffic for debugging, and optionally forward incoming webhooks to user-configured targets.

> Status: alpha preview. The repository already includes webhook signature verification for Midtrans and callback-token verification for Xendit, plus Midtrans and Xendit `pay create`, `pay status`, `pay refund`, `reconcile`, and SQLite persistence. Webhook forwarding target management is also available via CLI.

Latest alpha release: [v0.1.0-alpha.3](https://github.com/pendig/rute-bayar/releases/tag/v0.1.0-alpha.3)

## Features

- Modular provider adapters.
- CLI-first onboarding and operations.
- Webhook daemon per provider.
- Pass-through webhook forwarding.
- Raw inbound and outbound JSON storage for debugging and audit.
- SQLite-first local storage.
- Initial support target: Xendit Payment Sessions and Midtrans.

## Quick Start

Clone the repository:

```bash
git clone git@github.com:pendig/rute-bayar.git
cd rute-bayar
```

Install Go 1.22 or newer, then check the CLI:

```bash
go run ./cmd/rute-bayar version
go run ./cmd/rute-bayar provider list
```

Onboard Xendit credentials into local SQLite:

```bash
rute-bayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
rute-bayar provider accounts
```

Onboard Midtrans credentials into local SQLite:

```bash
rute-bayar onboard midtrans --merchant-id "$MIDTRANS_MERCHANT_ID" --client-key "$MIDTRANS_CLIENT_KEY" --server-key "$MIDTRANS_SERVER_KEY" --environment sandbox
rute-bayar provider test midtrans
```

Start the webhook daemon:

```bash
go run ./cmd/rute-bayar webhook serve --addr :8080 --environment sandbox
```

Check the daemon:

```bash
curl http://localhost:8080/healthz
```

### Local Webhook + Health Check

Run daemon and verify:

```bash
go run ./cmd/rute-bayar webhook serve --addr :8080 --environment sandbox
curl -i http://localhost:8080/healthz
```

Expected:

```json
{"status":"ok"}
```

Send a local webhook simulation:

```bash
curl -X POST http://localhost:8080/webhooks/xendit \
  -H 'Content-Type: application/json' \
  -d '{"event":"payment_session.created","status":"ACTIVE"}'
```

### Cloudflare Tunnel Test (temporary)

If you need a public URL for provider callback testing:

```bash
wrangler tunnel quick-start http://localhost:8080
```

After Cloudflare prints a public URL (for example `https://xxxx.trycloudflare.com`), verify:

```bash
curl -i https://xxxx.trycloudflare.com/healthz
```

Set provider webhook URL to:

```text
https://xxxx.trycloudflare.com/webhooks/xendit
https://xxxx.trycloudflare.com/webhooks/midtrans
```

The daemon verifies webhook signatures when provider credentials/configuration support it:
- Midtrans: `signature_key` is validated with `order_id + status_code + gross_amount + server_key`.
- Xendit: callback token validation uses `X-Callback-Token` when configured on onboarding.

Note: if the provider credentials/configuration are not present, webhook verification is skipped and requests are stored as raw inbound payloads for debugging.

## Installation

Build a local binary:

```bash
go build -o bin/rute-bayar ./cmd/rute-bayar
```

Run it:

```bash
./bin/rute-bayar version
```

Install into your Go binary path:

```bash
go install github.com/pendig/rute-bayar/cmd/rute-bayar@latest
```

For alpha builds, prefer the latest tagged release or a local build from `main`.

## Usage

Available command skeleton:

```bash
rute-bayar onboard
rute-bayar onboard xendit --secret-key <key> --environment sandbox
rute-bayar onboard midtrans --merchant-id <id> --client-key <key> --server-key <key> --environment sandbox
rute-bayar provider list
rute-bayar provider accounts
rute-bayar provider test midtrans
rute-bayar provider test xendit
rute-bayar pay create --provider xendit --method payment_link --reference rb-xnd-001 --amount 15000
rute-bayar pay create --provider midtrans --method bank_transfer --bank bca --reference rb-0001 --amount 15000
rute-bayar pay status --provider midtrans --reference rb-0001
rute-bayar pay refund
rute-bayar webhook serve --addr :8080
rute-bayar webhook forward list
rute-bayar webhook forward add
rute-bayar webhook forward update
rute-bayar webhook forward remove
rute-bayar webhook replay --event-id <id> [--provider midtrans|xendit]
rute-bayar webhook forward attempts list --status failed
rute-bayar webhook forward attempts show <attempt-id>
rute-bayar webhook forward attempts retry <attempt-id>
rute-bayar db migrate
rute-bayar reconcile
rute-bayar version
```

These commands establish the current user experience for alpha internal usage.

## Configuration

Copy the example environment file:

```bash
cp .env.example .env
```

Default local configuration:

```env
RUTE_BAYAR_ENV=sandbox
RUTE_BAYAR_DB_PATH=./rute-bayar.sqlite3
RUTE_BAYAR_WEBHOOK_ADDR=:8080
```

Do not commit `.env` or provider credentials. The file is ignored by Git.

### Troubleshooting

- `bind: operation not permitted` when starting daemon: environment may block local socket binding; try another port or run in a normal local terminal.
- `502` from `trycloudflare.com`: confirm the local daemon is running and still reachable at the forwarded local URL.
- DNS resolve failure for `*.trycloudflare.com`: usually environment/network-restricted; retry in another network/tool environment.

## Development

Run formatting and tests:

```bash
gofmt -w ./cmd ./internal
go test ./...
./scripts/smoke-local.sh
```

Validate the SQLite migration:

```bash
sqlite3 :memory: ".read migrations/0001_initial.sql"
```

Project layout:

- `cmd/rute-bayar`: CLI entrypoint.
- `internal/cli`: command routing.
- `internal/daemon`: HTTP daemon for webhook receiving.
- `internal/domain`: provider-neutral domain types.
- `internal/provider`: provider adapter contracts and registry.
- `internal/forwarding`: pass-through webhook forwarding service.
- `internal/storage`: storage implementations.
- `migrations`: SQLite schema migrations.
- `docs`: product and technical documentation.

Run the initial migration through the CLI:

```bash
rute-bayar db migrate
```

## Provider Notes

Xendit sandbox simulation has been tested with Payment Sessions:

- `POST /sessions` creates a Payment Session.
- `GET /sessions/{session_id}` retrieves status.
- Initial Xendit `ACTIVE` status maps naturally to Rute Bayar `pending`.
- `items[].category` is required for the tested Payment Session payload.

See [docs/xendit-sandbox-simulation.md](./docs/xendit-sandbox-simulation.md).

## Design Principles

- Modular per provider.
- Keep provider-specific behavior inside adapter packages.
- Store inbound and outbound payloads as raw JSON.
- Keep webhook forwarding pass-through by default.
- Make CLI onboarding simple before asking users to configure providers manually.
- Start with SQLite, but keep the domain portable.

## Documentation

Read the project docs:

- [Product Requirements](./docs/prd.md)
- [Architecture](./docs/architecture.md)
- [Model Data](./docs/data-model.md)
- [CLI Onboarding](./docs/cli-onboarding.md)
- [Provider Integration](./docs/provider-integration.md)
- [Implementation Status](./docs/implementation-status.md)
- [Webhook Forwarding](./docs/webhook-forwarding.md)
- [Status Mapping](./docs/status-mapping.md)
- [Operations Runbook](./docs/operations-runbook.md)
- [End-to-End Smoke Test](./docs/end-to-end-smoke.md)
- [Development](./docs/development.md)
- [Changelog](./CHANGELOG.md)
- [Xendit Sandbox Simulation](./docs/xendit-sandbox-simulation.md)
- [Midtrans Sandbox Simulation](./docs/midtrans-sandbox-simulation.md)

## Community

- [Contributing](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)
- [Support](./SUPPORT.md)
- [Issues](https://github.com/pendig/rute-bayar/issues)
- [Releases](https://github.com/pendig/rute-bayar/releases)

## License

Rute Bayar is released under the [MIT License](./LICENSE).

Copyright (c) 2026 Wahyu Adi Putra Pena Digital.

## Roadmap

- Stabilize Midtrans refund E2E when sandbox payable balance is available.
- Add more Midtrans payment methods and provider-specific diagnostics.
- Improve operational observability for webhook forwarding and replay.
- Prepare stable `v0.1.0` once release-readiness checks are complete.
