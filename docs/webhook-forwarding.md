# Webhook Forwarding

## Tujuan

Webhook forwarding memungkinkan daemon Rute Bayar meneruskan webhook provider ke target milik user.

## Prinsip

- Pass-through.
- Payload diteruskan apa adanya dari provider.
- Forwarding diaktifkan dan dikelola lewat CLI.
- Retry policy default tersedia dan bisa diubah lewat CLI.
- Logging tetap menyimpan inbound dan outbound mentah untuk debugging.

## Use Case

- Menyalurkan event Midtrans atau Xendit ke service internal lain.
- Membuat integrasi event-driven tanpa menulis ulang receiver webhook di setiap sistem.
- Menjaga satu titik masuk webhook di Rute Bayar.

## Alur

1. Provider mengirim webhook ke daemon.
2. Daemon memverifikasi webhook dan menyimpan payload mentah.
3. Event diproses oleh application layer.
4. Jika forwarding aktif, daemon mengirim webhook yang sama ke target user.
5. Jika target gagal, retry dijalankan sesuai policy.

## Konfigurasi via CLI

User harus bisa mengatur:

- nama target forwarding
- provider yang dipantau
- event filter
- target URL
- credential atau header tujuan
- retry policy
- status aktif/nonaktif

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

