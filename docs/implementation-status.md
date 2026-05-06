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
- Midtrans `pay status` untuk status inquiry order/VA.
- Xendit `pay create` untuk Payment Session.
- Persistence raw outbound request/response JSON untuk payment attempt.
- Persistence raw outbound request/response JSON untuk payment status check.
- Unit test untuk utility CLI, provider auth request, provider account storage, dan status mapping penting.
- Webhook signature verification untuk Midtrans (tergantung akun/onboarding key tersedia).
- Webhook callback token verification untuk Xendit (jika token konfigurasi diset).
- Webhook parsing event untuk payload Midtrans dan Xendit.
- Webhook reconciliation dasar: event parsed berhasil meng-update status `payment_intents` bila referensi cocok.
- Idempotency dasar: status webhook yang sama tidak mengulang update status ketika sudah sama.
- Forwarding target management lewat CLI (`webhook forward list|add|update|remove`) dengan penyimpanan konfigurasi di SQLite.
- Penyimpanan konfigurasi forward (headers, filter event, retry policy, enabled flag) dalam format JSON untuk kemudahan debugging.

## Belum Ada

- Xendit Payment Session refund.
- Midtrans Snap/Core adapter untuk create/refund dan perluasan metode lain.
- CI GitHub Actions.

## Command yang Sudah Ditargetkan

```bash
rute-bayar db migrate
rute-bayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
rute-bayar provider test xendit
rute-bayar onboard midtrans --merchant-id "$MIDTRANS_MERCHANT_ID" --client-key "$MIDTRANS_CLIENT_KEY" --server-key "$MIDTRANS_SERVER_KEY" --environment sandbox
rute-bayar provider test midtrans
rute-bayar pay create --provider midtrans --method bank_transfer --bank bca --reference rb-0001 --amount 15000
rute-bayar pay create --provider xendit --method payment_link --reference rb-xnd-001 --amount 15000
rute-bayar webhook forward add --provider midtrans --name orders --url https://example.com/webhooks/events --retry-max-attempts 4 --retry-timeout 10s --retry-backoff 2s
rute-bayar webhook forward list --provider midtrans
rute-bayar webhook forward update <target-id> --name orders-v2 --enabled=false
rute-bayar webhook forward remove <target-id>
```

## Catatan Verifikasi Lokal

Verifikasi terbaru:

- `healthz` lokal dan simulasi webhook sudah bisa di-check dari lingkungan pengujian yang memungkinkan socket local.
- `wrangler tunnel quick-start` untuk URL sementara bisa dipakai untuk simulasi callback publik.

Verifikasi tambahan yang tetap direkomendasikan:

```bash
sqlite3 :memory: ".read migrations/0001_initial.sql"
curl -i http://localhost:8080/healthz
```
