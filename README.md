# Rute Bayar

Rute Bayar adalah payment router berbasis Go untuk menjembatani provider payment gateway Indonesia seperti Midtrans dan Xendit.

Project ini dirancang sebagai:

- CLI untuk onboarding, operasi payment, status, refund, dan forwarding setup.
- Daemon untuk webhook receiver, verification, reconciliation, dan webhook forwarding.

## Prinsip

- Modular per provider.
- Simpan inbound dan outbound sebagai JSON mentah.
- Webhook forwarding pass-through.
- Konfigurasi forwarding bisa diatur lewat CLI.
- SQLite dipakai sebagai storage awal.

## Dokumentasi

Lihat folder [docs](./docs) untuk dokumen desain awal:

- [PRD](./docs/prd.md)
- [Arsitektur](./docs/architecture.md)
- [Model Data](./docs/data-model.md)
- [CLI Onboarding](./docs/cli-onboarding.md)
- [Provider Integration](./docs/provider-integration.md)
- [Webhook Forwarding](./docs/webhook-forwarding.md)
- [Development](./docs/development.md)

## Struktur Awal

- `cmd/rute-bayar`: entrypoint CLI.
- `internal/cli`: command routing CLI.
- `internal/daemon`: HTTP daemon untuk webhook.
- `internal/domain`: model internal netral provider.
- `internal/provider`: kontrak adapter provider.
- `internal/forwarding`: webhook forwarding pass-through.
- `migrations`: skema SQLite awal.

## Rekomendasi Teknologi

- Bahasa: Go
- Database: SQLite
- Distribusi: single binary

## Konfigurasi Lokal

Contoh environment tersedia di [.env.example](./.env.example).

## License

Open source under the MIT License. See [LICENSE](./LICENSE).
