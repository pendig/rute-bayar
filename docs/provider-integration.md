# Provider Integration

## Tujuan

Dokumen ini menjadi panduan untuk menambahkan provider baru ke Rute Bayar tanpa mengganggu core logic.

## Standar Integrasi

Setiap provider adapter wajib menyediakan:

- create payment
- get payment status
- refund payment
- verify webhook
- parse webhook event
- map status provider ke status internal
- capability declaration

## Mapping Status

Core system harus punya status netral provider, misalnya:

- `pending`
- `paid`
- `failed`
- `expired`
- `cancelled`
- `refunded`
- `partial_refunded`
- `settled`
- `authorized`
- `captured`

Provider-specific status harus selalu dipetakan ke status internal ini.

### Mapping Awal Midtrans

- `pending` -> `pending`
- `settlement` -> `settled`
- `capture` + `fraud_status: accept` -> `captured`
- `capture` + fraud status selain `accept` -> `pending`
- `deny` -> `failed`
- `failure` -> `failed`
- `cancel` -> `cancelled`
- `expire` -> `expired`
- `refund` -> `refunded`
- `partial_refund` -> `partial_refunded`

Catatan: mapping `settlement -> settled` dipilih agar status provider tetap informatif. Jika consumer butuh status bisnis yang lebih sederhana, application layer bisa memperlakukan `settled` sebagai paid-like final state.

### Mapping Awal Xendit

- Payment Session `ACTIVE` -> `pending`

Mapping Xendit lain perlu dilengkapi saat implementasi create/status Payment Session masuk.

### Mapping Awal DOKU

- Checkout `ORDER_GENERATED` -> `pending`
- Transaction `PENDING` -> `pending`
- Transaction `SUCCESS` -> `paid`
- Transaction `FAILED` -> `failed`
- Transaction `EXPIRED` -> `expired`
- Transaction `REFUNDED` -> `refunded`
- Transaction `PARTIAL_REFUNDED` -> `partial_refunded`

### Xendit `pay create`

- Endpoint aktif: `POST /sessions` (Payment Session API).
- `CreatePaymentRequest` dipetakan ke payload `reference_id`, `session_type=PAY`, `mode=PAYMENT_LINK`, `amount`, `currency`, `country`, `items[]`, `customer`.
- `reference_id` diisi dari `external reference`.
- `items[].category` dan `items[].type` wajib sesuai simulasi untuk menghindari validasi gagal.
- Response status awal umumnya `ACTIVE` dan dipetakan ke `pending`.
- URL pembayaran diambil dari `payment_link_url` dan ditampilkan sebagai `redirect_url`.

### DOKU Checkout `pay create`

- Endpoint aktif: `POST /checkout/v1/payment`.
- `CreatePaymentRequest` dipetakan ke payload `order.amount`, `order.invoice_number`, `order.currency`, dan `payment.payment_due_date`.
- `--method checkout` membiarkan DOKU Checkout menampilkan metode pembayaran aktif dari dashboard.
- `--notification-url` dikirim sebagai `additional_info.override_notification_url`.
- Request ditandatangani dengan header `Client-Id`, `Request-Id`, `Request-Timestamp`, `Digest`, dan `Signature`.
- URL pembayaran diambil dari `response.payment.url` dan ditampilkan sebagai `redirect_url`.

## Webhook Handling

Untuk setiap provider:

1. Verifikasi webhook sesuai mekanisme resmi provider.
2. Simpan payload mentah dan headers mentah.
3. Cek dedup/idempotency.
4. Normalisasi event.
5. Update payment state.
6. Jika forwarding aktif, kirim payload asli ke target user.

## Reconciliation

Webhook tidak boleh jadi satu-satunya sumber kebenaran.

Jika webhook gagal, terlambat, atau status belum final, sistem harus bisa:

- mengecek status lewat API provider
- membandingkan hasilnya dengan state internal
- memperbarui state bila ada perubahan

## JSON Logging Standard

Untuk debugging, semua provider harus menyimpan:

- outbound request JSON
- outbound response JSON
- inbound webhook JSON
- inbound webhook headers JSON

### Midtrans `pay create`

Implementasi awal `pay create` untuk Midtrans memakai Core API bank transfer:

- `payment_type=bank_transfer`
- `transaction_details.order_id` berasal dari external reference internal
- `transaction_details.gross_amount` berasal dari amount request
- `bank_transfer.bank` berasal dari bank/channel yang dipilih user

Untuk validasi webhook sandbox, `pay create` mendukung override URL notifikasi Midtrans per transaksi:

```bash
rutebayar pay create \
  --provider midtrans \
  --method qris \
  --bank gopay \
  --reference rb-midtrans-qris-001 \
  --amount 15000 \
  --notification-url https://<public-domain>/webhooks/midtrans
```

Nilai tersebut dikirim sebagai header `X-Override-Notification` pada request Midtrans Core API.

### Xendit Payment Sessions webhook URL

Untuk Xendit Payment Sessions, `pay create --notification-url` tidak didukung karena dokumentasi resmi Xendit tidak menyediakan override webhook per transaksi.
Konfigurasikan webhook/callback URL di Xendit Dashboard ke endpoint daemon:

```text
https://<public-domain>/webhooks/xendit
```

Jika payload perlu diteruskan ke aplikasi lain, gunakan fitur forwarding Rute Bayar agar webhook tetap masuk, tersimpan, diverifikasi, dan bisa direplay dari daemon.

Adapter harus menyimpan raw request dan raw response JSON ke payment attempt.

### DOKU Checkout webhook URL

Konfigurasikan notification URL DOKU ke endpoint daemon:

```text
https://<public-domain>/webhooks/doku
```

Rute Bayar memverifikasi `Signature` DOKU dengan target path webhook, digest body, client ID, request ID, timestamp, dan secret key.

Untuk per-payment override, gunakan:

```bash
rutebayar pay create \
  --provider doku \
  --method checkout \
  --reference rb-doku-001 \
  --amount 15000 \
  --notification-url https://<public-domain>/webhooks/doku
```

Refund DOKU belum diaktifkan karena membutuhkan setup Refund API/disbursement.

## Forwarding Policy

- Forwarding bersifat pass-through.
- Payload yang diforward tetap apa adanya dari provider.
- Retry policy default berlaku jika user tidak mengubahnya via CLI.
- Provider adapter tidak perlu tahu target forwarding; logic forwarding berada di daemon/application layer.

## Tambahan Provider Baru

Kalau provider baru ditambahkan nanti, langkah minimum:

1. Tambah adapter baru.
2. Tambah capability registry.
3. Tambah mapping status.
4. Tambah webhook endpoint.
5. Tambah onboarding flow di CLI.
6. Tambah test sandbox.
