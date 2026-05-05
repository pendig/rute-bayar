# Implementation Status

Dokumen ini mencatat status implementasi teknis Rute Bayar agar contributor mudah melihat bagian yang sudah ada dan bagian yang masih direncanakan.

## Sudah Ada

- Go module dan struktur package awal.
- CLI command routing.
- SQLite migration awal.
- SQLite store untuk provider account, webhook event, dan forwarding attempt.
- Config loader sederhana dari environment variable dan `.env`.
- Webhook daemon dasar.
- Webhook forwarding service dasar.
- Xendit onboarding ke SQLite.
- Xendit provider auth test via `GET /balance`.
- Midtrans sandbox simulation untuk Snap API dan Core API.
- Midtrans onboarding ke SQLite.
- Midtrans provider auth test via status inquiry order dummy.
- Midtrans `pay create` untuk Core API bank transfer.
- Persistence raw outbound request/response JSON untuk payment attempt.
- Unit test untuk utility CLI, provider auth request, provider account storage, dan status mapping penting.

## Belum Ada

- Xendit Payment Session adapter untuk create/status/refund.
- Midtrans Snap/Core adapter untuk status/refund dan perluasan metode lain.
- Webhook verification untuk Xendit.
- Webhook signature verification untuk Midtrans.
- Webhook event parsing dan status update internal.
- Forwarding target management yang persist lewat CLI.
- CI GitHub Actions.

## Command yang Sudah Ditargetkan

```bash
rute-bayar db migrate
rute-bayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
rute-bayar provider test xendit
rute-bayar onboard midtrans --merchant-id "$MIDTRANS_MERCHANT_ID" --client-key "$MIDTRANS_CLIENT_KEY" --server-key "$MIDTRANS_SERVER_KEY" --environment sandbox
rute-bayar provider test midtrans
rute-bayar pay create --provider midtrans --method bank_transfer --bank bca --reference rb-0001 --amount 15000
```

## Catatan Verifikasi Lokal

Saat dokumen ini ditulis, environment kerja belum memiliki Go toolchain di PATH. Karena itu `go test ./...` belum bisa dijalankan lokal dari sesi ini.

Verifikasi yang sudah bisa dilakukan:

```bash
sqlite3 :memory: ".read migrations/0001_initial.sql"
```
