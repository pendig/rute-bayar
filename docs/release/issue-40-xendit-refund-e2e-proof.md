# Issue #40 Xendit Refund E2E Proof Execution Log

Tanggal: 2026-05-16

## Tujuan
Membuktikan refund sandbox berhasil end-to-end untuk transaksi yang sudah settled/refundable, termasuk callback final dari provider, update row `refunds`, dan update status payment intent lokal.

## Setup
- Branch/source: `main` setelah PR #44 merge, dengan patch lokal refund webhook reconciliation.
- Database E2E: `/tmp/rb-xendit-real-webhook/rute-bayar.sqlite3`.
- Daemon: `127.0.0.1:18083`.
- Public tunnel: `https://relaxation-pixel-classroom-gay.trycloudflare.com`.
- Health check public: `GET /healthz` sukses.
- Xendit Dashboard sandbox/development disetel ke:
  - `https://relaxation-pixel-classroom-gay.trycloudflare.com/webhooks/xendit`
- Xendit callback token diverifikasi oleh daemon.

## Payment yang Direfund
- Provider: Xendit sandbox/development.
- Method: Payment Sessions `PAYMENT_LINK`.
- Payment reference: `rb-xendit-webhook-20260516204840`.
- Payment Session ID: `ps-6a0875bd0168694c2c30fd92`.
- Payment Request ID: `pr-98b84a81-e494-4703-9237-5ca60e3968bf`.
- Amount: `15000 IDR`.
- Status sebelum refund: `settled`.

## Refund
`pay refund` dijalankan terhadap payment yang sudah settled:

```text
provider: xendit
reference: rb-xendit-webhook-20260516204840
provider_reference: ps-6a0875bd0168694c2c30fd92
refund_reference: rb-refund-xendit-202605162110
status: pending
payment_request_id: pr-98b84a81-e494-4703-9237-5ca60e3968bf
```

Xendit mengembalikan status awal `PENDING`, lalu final status diterima melalui webhook.

## Webhook Final
Webhook final yang masuk:

```text
provider_event_id: refund.succeeded:rfd-fddd9d2e-5fc1-43e9-b08e-aef4c6a99f4b
event_type: refund.succeeded
processing_status: reconciled
signature_valid: 1
received_at: 2026-05-16T16:27:44Z
```

Payload final tersanitasi:

```text
id: rfd-fddd9d2e-5fc1-43e9-b08e-aef4c6a99f4b
amount: 15000
currency: IDR
status: SUCCEEDED
reference_id: rb-refund-xendit-202605162110
payment_request_id: pr-98b84a81-e494-4703-9237-5ca60e3968bf
payment_method_type: EWALLET
channel_code: DANA
```

## Hasil Lokal
Row `refunds` setelah callback:

```text
provider: xendit
provider_reference: ps-6a0875bd0168694c2c30fd92
amount: 15000
status: refunded
```

Payment intent lokal setelah callback:

```text
rb-xendit-webhook-20260516204840 | 15000 | IDR | refunded
```

## Kesimpulan
Xendit refund E2E untuk Issue #40 sudah terbukti:
- refund request dikirim ke provider.
- final refund callback real masuk ke daemon melalui public tunnel.
- callback token valid (`signature_valid=1`).
- event `refund.succeeded` diproses dan merekonsiliasi row `refunds`.
- payment intent lokal berubah menjadi `refunded`.

Catatan: selama eksekusi, Xendit Dashboard juga mengirim beberapa sample/test webhook yang tidak cocok dengan refund lokal dan diproses sebagai `unmatched`. Event final yang menutup proof adalah `refund.succeeded:rfd-fddd9d2e-5fc1-43e9-b08e-aef4c6a99f4b`.
