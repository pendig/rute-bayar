# Issue #39 Xendit Webhook Proof Execution Log

Tanggal: 2026-05-16

## Tujuan
Membuktikan callback real Xendit sandbox untuk Payment Sessions masuk ke daemon Rute Bayar melalui public tunnel dan memperbarui status lokal.

## Catatan Resmi Provider
Untuk Payment Sessions, webhook dikonfigurasi dari Xendit Dashboard. Xendit mengirim token verifikasi melalui header `X-Callback-Token`, dan daemon Rute Bayar memvalidasi header tersebut jika token disimpan saat onboarding.

Endpoint API `POST /callback_urls/{type}` tidak dipakai untuk proof ini karena dokumentasi Xendit menyatakan endpoint tersebut untuk Legacy Payment/Payout API tertentu, bukan konfigurasi webhook Payment Sessions modern.

## Setup
- Branch/source: `main`.
- Database E2E: `/tmp/rb-xendit-real-webhook/rute-bayar.sqlite3`.
- Daemon: `127.0.0.1:18082`.
- Public tunnel: `https://entertaining-rely-tested-cattle.trycloudflare.com`.
- Health check public: `GET /healthz` sukses.
- Xendit Dashboard sandbox/development disetel ke:
  - `https://entertaining-rely-tested-cattle.trycloudflare.com/webhooks/xendit`
- Xendit callback token disimpan saat onboarding dan diverifikasi oleh daemon.

## Payment
- Provider: Xendit sandbox/development.
- Method: Payment Sessions `PAYMENT_LINK`.
- Reference: `rb-xendit-webhook-20260516204840`.
- Payment Session ID: `ps-6a0875bd0168694c2c30fd92`.
- Hosted payment URL:
  - `https://dev.xen.to/cVukocUG`

Payment dibuat dengan `pay create`, lalu dibayar lewat hosted checkout Xendit sandbox.

## Hasil
`pay status` setelah pembayaran:

```text
provider: xendit
reference: rb-xendit-webhook-20260516204840
provider_reference: ps-6a0875bd0168694c2c30fd92
status: settled
status_message: COMPLETED
order_id: rb-xendit-webhook-20260516204840
payment_type: PAYMENT_LINK
redirect_url: https://dev.xen.to/cVukocUG
```

Webhook event tersimpan:

```text
provider_id: provider_xendit
provider_event_id: payment_session.completed:py-fcba2209-d185-47a9-a36f-edc5d6b9a18d
event_type: payment_session.completed
processing_status: reconciled
signature_valid: 1
received_at: 2026-05-16T13:50:55Z
```

Payment intent lokal:

```text
rb-xendit-webhook-20260516204840 | 15000 | IDR | settled
```

## Kesimpulan
Xendit path untuk Issue #39 sudah terbukti:
- provider callback real masuk ke daemon melalui public tunnel.
- callback token valid (`signature_valid=1`).
- event `payment_session.completed` diproses dan merekonsiliasi status lokal menjadi `settled`.

Dengan bukti Midtrans dan Xendit, Issue #39 sudah complete.
