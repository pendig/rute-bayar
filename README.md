# Rute Bayar

Rute Bayar is an open source payment router for Indonesian payment gateways.

The project provides one internal interface for multiple providers, starting with **Xendit** and **Midtrans**. It is designed as a Go CLI and daemon that can create payments, receive provider webhooks, store raw JSON traffic for debugging, and optionally forward incoming webhooks to user-configured targets.

> Status: early scaffold. The repository already contains the project structure, domain contracts, daemon skeleton, forwarding skeleton, docs, and initial SQLite migration. Provider API implementations are still in progress.

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
go run ./cmd/rute-bayar webhook serve --addr :8080
```

Check the daemon:

```bash
curl http://localhost:8080/healthz
```

Send a local webhook simulation:

```bash
curl -X POST http://localhost:8080/webhooks/xendit \
  -H 'Content-Type: application/json' \
  -d '{"event":"payment_session.created","status":"ACTIVE"}'
```

The current daemon accepts the webhook and runs the forwarding scaffold. Provider verification and persistence are planned next.

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

For now, prefer local builds from the repository until the first tagged release is published.

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
rute-bayar pay create --provider midtrans --method bank_transfer --bank bca --reference rb-0001 --amount 15000
rute-bayar pay status
rute-bayar pay refund
rute-bayar webhook serve --addr :8080
rute-bayar webhook replay
rute-bayar webhook forward list
rute-bayar webhook forward add
rute-bayar webhook forward update
rute-bayar webhook forward remove
rute-bayar db migrate
rute-bayar reconcile
rute-bayar version
```

These commands establish the intended user experience. The next implementation milestone is wiring them to SQLite and provider adapters.

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

## Development

Run formatting and tests:

```bash
gofmt -w ./cmd ./internal
go test ./...
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
- [Development](./docs/development.md)
- [Xendit Sandbox Simulation](./docs/xendit-sandbox-simulation.md)
- [Midtrans Sandbox Simulation](./docs/midtrans-sandbox-simulation.md)

## Roadmap

- Implement SQLite storage layer.
- Wire CLI onboarding to provider account storage.
- Implement Xendit Payment Session create/status/refund.
- Implement Xendit webhook verification and parsing.
- Implement Midtrans create/status/refund and notification handling.
- Persist raw inbound/outbound JSON for every provider operation.
- Add webhook forwarding target management via CLI.

## Contributing

Issues, ideas, and pull requests are welcome. For larger changes, please open an issue first so the design can stay aligned with the provider adapter model.

Before opening a pull request:

- Run `gofmt -w ./cmd ./internal`.
- Run `go test ./...`.
- Avoid committing provider credentials, `.env`, local SQLite files, or raw secret-bearing payloads.

## License

Rute Bayar is open source under the [MIT License](./LICENSE).

Copyright (c) 2026 Wahyu Adi Putra Pena Digital.
