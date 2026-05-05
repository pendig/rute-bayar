# Development

## Prerequisites

- Go 1.22 atau lebih baru.
- SQLite untuk storage lokal.

## Setup Awal

Project sudah disiapkan sebagai Go module:

```bash
go mod tidy
go test ./...
```

Catatan: environment saat scaffold awal dibuat belum memiliki `go` di PATH, jadi verifikasi compile perlu dijalankan setelah Go tersedia.

## Command Awal

```bash
go run ./cmd/rute-bayar version
go run ./cmd/rute-bayar provider list
go run ./cmd/rute-bayar webhook serve --addr :8080
go run ./cmd/rute-bayar db migrate
```

## Migration

Skema SQLite awal ada di:

```text
migrations/0001_initial.sql
```

Migration ini mencakup:

- providers
- provider accounts
- payment intents
- payment attempts
- webhook events
- webhook forwarding targets
- webhook forwarding attempts
- refunds
- audit logs

## Catatan Implementasi Berikutnya

- Tambahkan driver SQLite untuk Go.
- Implementasikan storage SQLite berdasarkan migration awal.
- Hubungkan CLI onboarding ke storage SQLite.
- Implementasikan adapter Midtrans dan Xendit sesuai dokumentasi resmi provider.
