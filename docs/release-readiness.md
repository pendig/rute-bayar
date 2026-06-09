# Release Readiness Checklist

Dokumen ini dipakai sebagai checklist sebelum Rute Bayar ditandai sebagai release publik, misalnya `v0.1.0`.

## Status Saat Ini

Rute Bayar sudah layak untuk:

- demo internal
- alpha / preview tertutup
- uji integrasi provider secara manual
- stable `v0.1.0`

Rute Bayar sudah memiliki bukti real provider webhook callback dan Xendit refund E2E di sandbox. Refund tetap capability-specific; iPaymu refund belum tersedia karena API publik iPaymu v2 belum mengekspos endpoint refund resmi/terverifikasi.
Checklist, changelog, dan release notes sudah disiapkan untuk release publik stabil `v0.1.0`.

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
- Real webhook callback proof untuk Midtrans dan Xendit.
- Xendit refund E2E proof dari request refund sampai callback `refund.succeeded` dan status lokal `refunded`.

## Final Pass `v0.1.0`

### Core Payments

- Konfirmasi `pay create`, `pay status`, dan `pay refund` tetap hijau lewat CI dan smoke test untuk provider yang mendukung refund. Jangan jadikan ketiadaan refund iPaymu sebagai blocker rilis sampai ada endpoint resmi dari iPaymu.
- Pastikan status mapping yang terdokumentasi masih sesuai dengan adapter.
- Midtrans refund E2E masih dapat dicatat sebagai known sandbox limitation jika balance sandbox tidak tersedia.

### Webhook

- Verifikasi ulang bukti Midtrans dan Xendit real webhook callback di `docs/release/`.
- Verifikasi ulang Xendit refund callback proof di `docs/release/issue-40-xendit-refund-e2e-proof.md`.
- Pastikan idempotency webhook tetap aman untuk retry.

### Forwarding

- Jalankan smoke test forwarding lokal.
- Pastikan diagnostic command untuk forwarding attempts masih terdokumentasi.
- Pastikan `docs/operations-runbook.md` menjelaskan replay dan retry failure path.

### Operasional

- `go test ./...` harus hijau di local dan CI.
- `go vet ./...` harus hijau sebelum tag stable.
- Pastikan tidak ada secret provider tersimpan di repo.

### Release Engineering

#### Wajib untuk setiap update versi (tag `v*`)

- Update `CHANGELOG.md` dengan entri penuh (added/changed/fixed).
- Update `site/src/data/changelog.ts` agar halaman `/changelog/` ikut sinkron.
- Update `README.md` release pointer/status text jika diperlukan.
- Update dokumentasi publik di `docs/` untuk fitur/behavior yang berubah.
- Simpan atau perbarui bukti operasi di `docs/release/` (termasuk issue execution log kalau ada).

Semua poin di atas harus dicentang saat PR memuat perubahan untuk rilis versi.

- Update changelog/release notes untuk `v0.1.0`.
- Pastikan README tidak lagi memberi kesan alpha setelah stable tag siap dibuat.
- Release artifact automation yang repeatable.
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
