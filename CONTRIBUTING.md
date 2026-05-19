# Contributing to Rute Bayar

Thank you for considering a contribution to Rute Bayar. The project aims to be a practical, maintainable payment router for Indonesian payment gateways.

## Ways to Contribute

- Report bugs with clear reproduction steps.
- Propose provider integrations or payment methods.
- Improve documentation, examples, and operational runbooks.
- Add tests around payment, webhook, forwarding, and storage behavior.
- Review pull requests with a focus on correctness and maintainability.

## Development Setup

Requirements:

- Go 1.22 or newer.
- SQLite tooling for local inspection.
- Provider sandbox credentials only when running E2E checks.

Run locally:

```bash
go build -o ./bin/rutebayar ./cmd/rute-bayar
./bin/rutebayar version
go test ./...
./scripts/smoke-local.sh
```

## Pull Request Checklist

Before opening a pull request:

- Run `gofmt -w ./cmd ./internal`.
- Run `go test ./...`.
- Update documentation when behavior or CLI usage changes.
- Do not commit `.env`, SQLite files, provider credentials, or raw secret-bearing payloads.
- Keep provider-specific behavior inside the relevant adapter package.
- Keep raw inbound/outbound JSON storage behavior intact unless the change is explicitly about storage semantics.

## Commit Style

Use short, imperative commit messages:

```text
feat: add midtrans qris payments
fix: preserve refunded status during provider inquiry
docs: add webhook tunnel runbook
```

## Provider Work

When adding or changing a provider feature:

- Prefer official provider documentation.
- Store raw request and response JSON for debugging.
- Normalize provider statuses through the internal domain status model.
- Add unit tests for request payloads, response parsing, and error handling.
- Document any sandbox limitations or manual testing steps.

## Security

Never include real credentials in issues, pull requests, logs, screenshots, or docs. If a secret is exposed, rotate it immediately.

For vulnerability reports, follow [SECURITY.md](./SECURITY.md).
