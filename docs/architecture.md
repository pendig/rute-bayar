# Architecture

## Gambaran Umum

Rute Bayar dibangun sebagai payment router berbasis **Go** dengan dua komponen utama:

- **CLI** untuk onboarding dan operasi manual.
- **Daemon** untuk webhook receiver dan background reconciliation.

## Prinsip Desain

- Modular per provider.
- Domain internal netral provider.
- Inbound dan outbound disimpan sebagai JSON mentah.
- Webhook forwarding bersifat pass-through, tanpa transformasi payload.
- Semua operasi penting dibuat idempotent.
- Provider-specific logic tidak bocor ke layer bisnis inti.

## Layer

### 1. CLI Layer

Menangani:

- onboarding provider
- setup credential
- create payment
- check status
- refund
- replay webhook
- reconcile transaksi

### 2. Application Layer

Menangani use case bisnis:

- create payment intent
- record payment attempt
- normalize provider response
- process webhook event
- sync status
- issue refund
- evaluate forwarding rule
- dispatch forward job

### 3. Provider Adapter Layer

Setiap provider punya adapter sendiri.

Contoh:

- `providers/midtrans`
- `providers/xendit`
- `providers/doku`

Tanggung jawab adapter:

- request mapping
- response mapping
- webhook verification
- webhook parsing
- supported capabilities

### 4. Persistence Layer

SQLite dipakai untuk:

- payment intents
- payment attempts
- webhook events
- refunds
- provider configuration
- audit logs

### 5. Daemon Layer

Daemon expose endpoint webhook per provider, misalnya:

- `/webhooks/midtrans`
- `/webhooks/xendit`
- `/webhooks/doku`

Fungsi daemon:

- verifikasi signature/token
- dedup webhook
- simpan payload mentah
- dispatch ke application layer
- update status transaksi
- forward webhook ke target yang sudah dikonfigurasi user

### 6. Forwarding Layer

Forwarding dibuat sebagai kemampuan daemon untuk mengirim ulang webhook inbound ke target pilihan user.

Karakteristik:

- pass-through tanpa transformasi payload
- header dan body diteruskan apa adanya sejauh memungkinkan
- konfigurasi target diset lewat CLI
- retry policy default tersedia dan bisa diubah lewat CLI
- logging tetap menyimpan request/response mentah

## Alur Data

### Create Payment

1. CLI atau internal API memanggil application layer.
2. Application layer memilih provider adapter.
3. Adapter membuat request outbound ke provider.
4. Request outbound disimpan sebagai JSON mentah.
5. Response provider disimpan sebagai JSON mentah.
6. Status internal dibuat atau diperbarui.

### Webhook

1. Provider mengirim webhook ke daemon.
2. Daemon memverifikasi signature/token.
3. Payload inbound disimpan sebagai JSON mentah.
4. Event di-normalisasi ke domain internal.
5. Status transaksi diperbarui.
6. Jika user mengaktifkan forwarding, payload dikirim ke target tujuan.
7. Jika event tidak final, reconciliation job dapat mengecek status provider.

## Observability

Minimal yang harus ada:

- structured logging
- request/response raw JSON
- audit log
- retry counter
- webhook delivery history
- forward delivery history

## Kemudahan Ekstensi

Kalau provider baru ditambahkan, perubahan idealnya hanya terjadi di:

- provider adapter baru
- capability registry
- mapping status
- webhook parser

Core domain dan CLI contract tidak perlu berubah besar.
