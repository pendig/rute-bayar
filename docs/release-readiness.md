# Release Readiness Checklist

Dokumen ini dipakai sebagai checklist sebelum Rute Bayar ditandai sebagai release publik, misalnya `v0.1.0`.

## Status Saat Ini

Rute Bayar sudah layak untuk:

- demo internal
- alpha / preview tertutup
- uji integrasi provider secara manual

Rute Bayar belum siap untuk release publik stabil karena beberapa komponen inti masih belum final.

## Sudah Siap

- CLI onboarding provider untuk Midtrans dan Xendit.
- Penyimpanan credential provider ke SQLite.
- `provider test` untuk Midtrans dan Xendit.
- `pay create` Midtrans Core API bank transfer.
- Penyimpanan raw JSON inbound dan outbound untuk debugging.
- `pay create` Xendit Payment Session sudah bisa digunakan.
- Dokumentasi produk, arsitektur, onboarding, dan integrasi provider.
- PR workflow dasar di GitHub.

## Wajib Selesai Sebelum `v0.1.0`

### Core Payments

- `refund` minimal untuk provider yang sudah didukung.
- Status mapping yang konsisten antar provider.

### Webhook

- Verifikasi signature Midtrans.
- Verifikasi webhook Xendit.
- Parsing webhook menjadi event internal.
- Update payment intent berdasarkan webhook.
- Idempotency webhook yang aman untuk retry.

### Forwarding

- CRUD target forwarding lewat CLI.
- Persist forwarding target di SQLite.
- Retry policy yang bisa dikonfigurasi dari CLI.
- Replay/diagnostic command untuk forwarding gagal.

### Operasional

- `go test ./...` harus bisa dijalankan di environment CI.
- GitHub Actions untuk lint/test minimal.
- Pastikan `gofmt` dan dependency checks konsisten.
- Pastikan tidak ada secret provider tersimpan di repo.

### Release Engineering

- Versi tag pertama `v0.1.0`.
- Changelog release.
- Binary build instruction yang sudah stabil.
- README yang jelas untuk install, konfigurasi, dan usage dasar.
- Validasi migrasi SQLite dari fresh install.

## Acceptance Criteria `v0.1.0`

Release pertama dianggap siap jika:

- CLI bisa onboard provider, test provider, dan membuat payment Midtrans serta Xendit tanpa manual patch.
- Webhook provider bisa diverifikasi dan diproses dengan benar.
- Raw inbound/outbound JSON tersimpan untuk audit dan debugging.
- Setidaknya satu provider end-to-end bekerja untuk create, receive webhook, dan cek status.
- Test otomatis jalan di CI.

## Rekomendasi Urutan Kerja

1. Ikuti [Implementation Plan](./implementation-plan.md).
2. Implement webhook verification dan parsing.
3. Tambahkan forwarding target CLI.
4. Tambahkan CI GitHub Actions.
5. Rapikan release notes dan tag `v0.1.0`.
