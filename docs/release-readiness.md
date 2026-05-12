# Release Readiness Checklist

Dokumen ini dipakai sebagai checklist sebelum Rute Bayar ditandai sebagai release publik, misalnya `v0.1.0`.

## Status Saat Ini

Rute Bayar sudah layak untuk:

- demo internal
- alpha / preview tertutup
- uji integrasi provider secara manual

Rute Bayar belum siap untuk release publik stabil sampai Issue #36 (real webhook + real refund E2E) tuntas.

## Sudah Siap

- CLI onboarding provider untuk Midtrans dan Xendit.
- Penyimpanan credential provider ke SQLite.
- `provider test` untuk Midtrans dan Xendit.
- `pay create` Midtrans Core API bank transfer.
- Penyimpanan raw JSON inbound dan outbound untuk debugging.
- `pay create` Xendit Payment Session sudah bisa digunakan.
- Dokumentasi produk, arsitektur, onboarding, dan integrasi provider.
- PR workflow dasar di GitHub.
- Webhook reconciliation dasar (status intent sinkron dari event yang tervalidasi).
- CI GitHub Actions untuk format check, vet, test, dan build matrix.
- Diagnostics forwarding attempt lewat CLI (`list`, `show`, `retry`).
- Smoke test lokal otomatis via `scripts/smoke-local.sh`.
- Status mapping provider terdokumentasi.

## Wajib Selesai Sebelum `v0.1.0`

### Core Payments

- `refund` minimal untuk provider yang sudah didukung.
- Status mapping yang konsisten antar provider.

### Webhook

- Verifikasi signature Midtrans.
- Verifikasi webhook Xendit.
- Parsing webhook menjadi event internal.
- Idempotency webhook yang aman untuk retry.

### Forwarding

- CRUD target forwarding lewat CLI.
- Persist forwarding target di SQLite.
- Retry policy yang bisa dikonfigurasi dari CLI.
- Replay command untuk forwarding gagal (`webhook replay`).
- Diagnostic command untuk forwarding attempts (`webhook forward attempts ...`).
- Event filter forwarding untuk seleksi event berdasarkan header/payload.
- Diagnostik operasional tersimpan di `docs/operations-runbook.md`.

### Operasional

- `go test ./...` harus bisa dijalankan di environment CI.
- GitHub Actions untuk lint/test minimal.
- Pastikan `gofmt`, `go vet`, dan dependency checks konsisten.
- Pastikan tidak ada secret provider tersimpan di repo.

### Release Engineering

- Versi tag pertama `v0.1.0`.
- Changelog release.
- Binary build instruction yang sudah stabil.
- Release artifact automation yang repeatable.
- README yang jelas untuk install, konfigurasi, dan usage dasar.
- Validasi migrasi SQLite dari fresh install.

## Release Automation

Release GitHub otomatis dibuat saat tag `v*` dipush.

Contoh:

```bash
git tag -a v0.1.0-alpha.2 -m "v0.1.0-alpha.2"
git push origin v0.1.0-alpha.2
```

Workflow release akan:

- menjalankan format check, `go vet`, dan `go test ./...`
- build binary Linux, macOS, dan Windows
- membuat `checksums.txt`
- publish GitHub Release
- menandai release sebagai prerelease jika tag mengandung `alpha`, `beta`, atau `rc`

## Acceptance Criteria `v0.1.0`

Release pertama dianggap siap jika:

- CLI bisa onboard provider, test provider, dan membuat payment Midtrans serta Xendit tanpa manual patch.
- Webhook provider bisa diverifikasi dan diproses dengan benar.
- Raw inbound/outbound JSON tersimpan untuk audit dan debugging.
- Setidaknya satu provider end-to-end bekerja untuk create, receive webhook, dan cek status.
- Test otomatis jalan di CI.
- Smoke test lokal dan checklist end-to-end tersedia.

## Rekomendasi Urutan Kerja

1. Ikuti [Implementation Plan](./implementation-plan.md).
2. Implement webhook verification dan parsing.
3. Tambahkan forwarding target CLI.
4. Kunci hardening webhook:
   - batasan retry policy yang eksplisit untuk failure class
   - pengamanan signature/token edge case
   - observability untuk retry/response body lengkap
5. Tambahkan CI GitHub Actions.
6. Rapikan release notes dan tag `v0.1.0`.
