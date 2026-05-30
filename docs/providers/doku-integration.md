# DOKU Integration

Dokumen ini mencatat implementasi provider **DOKU Checkout** di Rute Bayar.

## Status

- `pay create --provider doku` tersedia melalui DOKU Checkout.
- `pay status --provider doku` tersedia melalui DOKU Check Status API.
- Webhook `/webhooks/doku` tersedia dengan verifikasi signature DOKU.
- Webhook forwarding tetap pass-through, sama seperti provider lain.
- Semua outbound request/response dan inbound webhook/header tetap disimpan sebagai JSON.
- `pay refund --provider doku` belum diaktifkan karena refund DOKU membutuhkan konfigurasi Refund API/disbursement terpisah.

## Official References

- DOKU Checkout backend integration: `POST /checkout/v1/payment`.
  https://developers.doku.com/accept-payments/doku-checkout/integration-guide/backend-integration
- DOKU Check Status API: `GET /orders/v1/status/{invoice_number_or_request_id}`.
  https://developers.doku.com/get-started-with-doku-api/check-status-api/non-snap
- DOKU HTTP Notification and webhook setup.
  https://docs.doku.com/get-started/manage-business/set-up-integration/webhook-payment-notification
- DOKU signature sample and HMAC-SHA256 header format.
  https://dashboard.doku.com/docs/docs/technical-references/sample/go-signature-sample/
- DOKU refund policy and Refund API/disbursement context.
  https://docs.doku.com/accept-payments/finance-and-settlement/refund-and-chargeback/refund-and-chargeback

## Onboarding

```bash
rutebayar onboard doku \
  --client-id "$DOKU_CLIENT_ID" \
  --secret-key "$DOKU_SECRET_KEY" \
  --environment sandbox
```

Optional webhook path:

```bash
rutebayar onboard doku \
  --client-id "$DOKU_CLIENT_ID" \
  --secret-key "$DOKU_SECRET_KEY" \
  --webhook-path /webhooks/doku \
  --environment sandbox
```

`--webhook-path` dipakai untuk verifikasi signature webhook. Nilai default mengikuti endpoint daemon Rute Bayar: `/webhooks/doku`.

## Smoke Commands

```bash
rutebayar provider test doku --environment sandbox

rutebayar pay create \
  --provider doku \
  --method checkout \
  --reference rb-doku-001 \
  --amount 15000 \
  --notification-url https://<public-domain>/webhooks/doku

rutebayar pay status \
  --provider doku \
  --reference rb-doku-001
```

## Payment Methods

`--method checkout` menampilkan metode pembayaran aktif di DOKU Checkout.

Provider adapter juga mendukung filter metode DOKU Checkout berikut:

- `--method bank_transfer --bank bca`
- `--method bank_transfer --bank mandiri`
- `--method bank_transfer --bank bri`
- `--method bank_transfer --bank bni`
- `--method qris`
- `--method credit_card`
- `--method ewallet --bank ovo`
- `--method ewallet --bank dana`
- `--method convenience_store --bank alfa`
- raw DOKU method type, misalnya `--method VIRTUAL_ACCOUNT_BNI`

## Webhook Verification

DOKU mengirim header:

- `Client-Id`
- `Request-Id`
- `Request-Timestamp`
- `Signature`

Rute Bayar menghitung ulang signature dengan format:

```text
Client-Id:<client_id>
Request-Id:<request_id>
Request-Timestamp:<request_timestamp>
Request-Target:<webhook_path>
Digest:<base64_sha256_body>
```

Hasil HMAC-SHA256 dibandingkan dengan header `Signature` yang diawali `HMACSHA256=`.

## Status Mapping

- `ORDER_GENERATED`, `ORDER_RECOVERED`, `PENDING`, `REDIRECT`, `TIMEOUT` -> `pending`
- `SUCCESS`, `APPROVE` -> `paid`
- `FAILED`, `REJECT` -> `failed`
- `ORDER_EXPIRED`, `EXPIRED` -> `expired`
- `CANCELLED`, `CANCELED` -> `cancelled`
- `REFUNDED` -> `refunded`
- `PARTIAL_REFUNDED` -> `partial_refunded`

## Follow-Up

- Tambahkan DOKU refund setelah credential dan flow Refund API/disbursement disiapkan.
- Tambahkan E2E sandbox DOKU Checkout ketika akun sandbox DOKU tersedia.
- Tambahkan contoh webhook fixture DOKU dari dashboard/simulator resmi.
