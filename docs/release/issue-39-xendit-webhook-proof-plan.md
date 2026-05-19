# Issue #39 Xendit Webhook Proof Plan

Tanggal: 2026-05-16

## Tujuan
Membuktikan callback real Xendit sandbox untuk Payment Sessions masuk ke daemon Rute Bayar melalui public tunnel dan memperbarui status lokal.

## Catatan Resmi Provider
Untuk Payment Sessions, webhook dikonfigurasi dari Xendit Dashboard. Xendit mengirim token verifikasi melalui header `X-Callback-Token`, dan daemon Rute Bayar memvalidasi header tersebut jika token disimpan saat onboarding.

Endpoint API `POST /callback_urls/{type}` tidak dipakai untuk proof ini karena dokumentasi Xendit menyatakan endpoint tersebut untuk Legacy Payment/Payout API tertentu, bukan konfigurasi webhook Payment Sessions modern.

## Setup yang Dibutuhkan
- Daemon Rute Bayar berjalan pada localhost.
- Public tunnel aktif ke daemon.
- Xendit sandbox/development dashboard disetel ke URL:
  - `https://<public-domain>/webhooks/xendit`
- Xendit webhook verification token sama dengan `XENDIT_WEBHOOK_TOKEN` yang dipakai saat onboarding.

## Langkah Eksekusi
1. Build binary dari `main`.
2. Buat database E2E baru, misalnya:
   - `/tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3`
3. Jalankan:
   - `rutebayar db migrate --db /tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3`
   - `rutebayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --webhook-token "$XENDIT_WEBHOOK_TOKEN" --environment sandbox --db /tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3`
   - `rutebayar webhook serve --addr 127.0.0.1:<port> --environment sandbox --db /tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3`
4. Buka tunnel:
   - `cloudflared tunnel --url http://127.0.0.1:<port>`
5. Di Xendit dashboard sandbox/development, set webhook URL Payment Sessions ke:
   - `https://<public-domain>/webhooks/xendit`
6. Buat payment:
   - `rutebayar pay create --provider xendit --method payment_link --reference <ref> --amount 15000 --db /tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3`
7. Selesaikan payment dari hosted payment link sandbox Xendit.
8. Verifikasi:
   - `webhook_events` bertambah.
   - `signature_valid=1`.
   - settlement/completed event diproses.
   - `payment_intents.status` berubah sesuai webhook.
   - `rutebayar pay status --provider xendit --reference <ref> --db /tmp/rb-xendit-webhook-proof/rute-bayar.sqlite3` sinkron dengan provider.

## Evidence yang Harus Dicatat
- Public tunnel URL.
- Reference dan provider reference.
- Hosted payment URL.
- Sanitized rows dari `webhook_events`.
- Sanitized `payment_intents` final status.
- Output `pay status`.

## Status
Belum dieksekusi. Menunggu konfigurasi webhook URL di Xendit dashboard sandbox/development.
