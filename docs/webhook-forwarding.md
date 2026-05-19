# Webhook Forwarding

## Tujuan

Webhook forwarding memungkinkan daemon Rute Bayar meneruskan webhook provider ke target milik user.

## Prinsip

- Pass-through.
- Payload diteruskan apa adanya dari provider.
- Forwarding diaktifkan dan dikelola lewat CLI.
- Retry policy default tersedia dan bisa diubah lewat CLI.
- Logging tetap menyimpan inbound dan outbound mentah untuk debugging.
- Event filter membuat forwarding selektif agar operator hanya memproses event yang relevan.

## Use Case

- Menyalurkan event Midtrans atau Xendit ke service internal lain.
- Membuat integrasi event-driven tanpa menulis ulang receiver webhook di setiap sistem.
- Menjaga satu titik masuk webhook di Rute Bayar.

## Alur

1. Provider mengirim webhook ke daemon.
2. Daemon memverifikasi webhook dan menyimpan payload mentah.
3. Event diproses oleh application layer.
4. Jika forwarding aktif, daemon mengecek `event_filter` target lalu mengirim webhook yang sama ke target user.
5. Jika target gagal, retry dijalankan sesuai policy.

### Event Filter Semantik

`event_filter` disimpan sebagai key-value JSON dan dipasang lewat flag CLI:

```bash
--event-filter "event=payment"
--event-filter "X-Event-Type=payment_settlement"
```

Untuk setiap pasangan filter:

- Coba cocokkan dulu dari header inbound (nama header persis seperti yang dikirim provider).
- Jika tidak ada di header, cocokkan ke field top-level payload JSON.
- Semua filter harus match agar target dieksekusi.
- Perbandingan dilakukan case-insensitive terhadap string.

## Konfigurasi via CLI

User harus bisa mengatur:

- nama target forwarding
- provider yang dipantau
- event filter
- target URL
- credential atau header tujuan
- retry policy
- status aktif/nonaktif

Contoh tambahan:

```bash
rutebayar webhook forward add --provider midtrans --name orders \
  --url https://example.com/webhooks/orders \
  --event-filter "event=payment_session.created" \
  --event-filter "X-Event-Type=payment_settlement" \
  --retry-max-attempts 5 \
  --retry-timeout 10s \
  --retry-backoff 2s
```

## Default Retry Policy

Default policy yang disarankan:

- beberapa kali retry
- exponential backoff sederhana
- timeout per request yang wajar

Policy ini boleh diganti lewat CLI.

## Data yang Disimpan

- payload inbound mentah
- headers inbound mentah
- request outbound forwarding mentah
- response outbound forwarding mentah
- status attempt forwarding

## Replay Webhook

`webhook replay` menjalankan ulang forwarding dari event yang sudah tersimpan:

```bash
rutebayar webhook replay --event-id <event_id> --db ./rute-bayar.sqlite3
```

Optional filter provider:

```bash
rutebayar webhook replay --provider midtrans --event-id <event_id>
```

CLI ini berguna untuk:

- menguji konfigurasi forwarding tanpa menunggu event baru
- memastikan retry policy sudah berjalan setelah perubahan target
- simulasi lokal saat mode pengembangan

## Diagnostics Attempt

Lihat attempt terbaru:

```bash
rutebayar webhook forward attempts list --limit 20
```

Filter attempt gagal:

```bash
rutebayar webhook forward attempts list --status failed
```

Lihat raw request/response attempt:

```bash
rutebayar webhook forward attempts show <attempt_id>
```

Retry attempt manual ke target yang sama:

```bash
rutebayar webhook forward attempts retry <attempt_id>
```

Jika target sudah disabled, retry manual akan ditolak kecuali operator memakai `--force-disabled`.
