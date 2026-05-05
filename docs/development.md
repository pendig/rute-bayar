# Development

## Prerequisites

- Go 1.22 atau lebih baru.
- SQLite untuk storage lokal.

## Setup Awal

Pastikan project ter-setup dan bisa ditest:

```bash
go mod tidy
go test ./...
```

## Command Awal

```bash
go run ./cmd/rute-bayar version
go run ./cmd/rute-bayar provider list
go run ./cmd/rute-bayar webhook serve --addr :8080
go run ./cmd/rute-bayar db migrate
go run ./cmd/rute-bayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
go run ./cmd/rute-bayar provider accounts
go run ./cmd/rute-bayar provider test xendit
go run ./cmd/rute-bayar onboard midtrans --merchant-id "$MIDTRANS_MERCHANT_ID" --client-key "$MIDTRANS_CLIENT_KEY" --server-key "$MIDTRANS_SERVER_KEY" --environment sandbox
go run ./cmd/rute-bayar provider test midtrans
```

## Health Check Webhook Daemon

Jalankan daemon:

```bash
go run ./cmd/rute-bayar webhook serve --addr :8080 --environment sandbox
```

Di terminal lain, cek health endpoint:

```bash
curl -i http://localhost:8080/healthz
```

Respon sukses:

```json
{"status":"ok"}
```

Simulasi webhook lokal:

```bash
curl -i -X POST http://localhost:8080/webhooks/xendit \
  -H 'Content-Type: application/json' \
  -d '{"event":"payment_session.status.changed","status":"COMPLETED","reference_id":"INV-1001","id":"evt_001"}'
```

atau:

```bash
export MIDTRANS_ORDER_ID="ORD-1001"
export MIDTRANS_STATUS_CODE="200"
export MIDTRANS_GROSS_AMOUNT="10000"
export MIDTRANS_SERVER_KEY="$MIDTRANS_SERVER_KEY"
export MIDTRANS_SIGNATURE=$(
  printf '%s%s%s%s' \
    "$MIDTRANS_ORDER_ID" \
    "$MIDTRANS_STATUS_CODE" \
    "$MIDTRANS_GROSS_AMOUNT" \
    "$MIDTRANS_SERVER_KEY" \
  | openssl dgst -sha512 -hex \
  | awk '{print $2}'
)

curl -i -X POST http://localhost:8080/webhooks/midtrans \
  -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"$MIDTRANS_ORDER_ID\",\"status_code\":\"$MIDTRANS_STATUS_CODE\",\"gross_amount\":\"$MIDTRANS_GROSS_AMOUNT\",\"transaction_status\":\"capture\",\"fraud_status\":\"accept\",\"payment_type\":\"bank_transfer\",\"signature_key\":\"$MIDTRANS_SIGNATURE\",\"transaction_id\":\"trx_001\",\"transaction_time\":\"2026-05-05T00:00:00Z\"}"
```

Kedua endpoint di atas seharusnya mengembalikan `202 Accepted`.

## Troubleshooting Khusus Provider

### Midtrans

- Response `400 Bad Request` dengan error signature:
  - pastikan `server_key` sudah onboard untuk `sandbox/production` yang sama dengan daemon environment.
  - pastikan payload ada semua field berikut: `order_id`, `status_code`, `gross_amount`, `signature_key`.
  - pastikan nilai `gross_amount` di webhook sama persis (format string/numerik) dengan nilai yang dihitung `Midtrans`.
  - hitung ulang `signature_key` dengan `sha512(order_id + status_code + gross_amount + server_key)` (tanpa pemisah).
- Jika webhook `200/202` tidak masuk ke log parse:
  - cek apakah payload webhook sudah termasuk `transaction_status` dan `fraud_status` agar mapping status bisa lebih lengkap.
  - cek `handler` tidak terbentuk bila akun Midtrans belum di-onboard; di mode itu verification tidak akan jalan.

### Xendit

- Response `400 Bad Request` dengan `callback token`:
  - pastikan Xendit mengirim header `X-Callback-Token` bila token di-set saat onboarding.
  - jika tidak pakai token saat ini, hapus `--webhook-token` saat onboarding lalu restart daemon.
- Jika webhook 202 tapi tidak ada efek `payment_status`:
  - payload biasanya tidak punya `reference_id` atau `order_id`; gunakan `reference_id`/`external_id` yang sama dengan `create payment` reference.
  - status terbaru diterima di field `status` (contoh: `ACTIVE`, `COMPLETED`, `FAILED`, `EXPIRED`).

## Cloudflare Tunnel Testing

Untuk webhook test dari internet sementara, gunakan Cloudflare tunnel:

```bash
wrangler tunnel quick-start http://localhost:8080
```

Command di atas akan menampilkan URL seperti `https://xxxx.trycloudflare.com`.
Setelah URL muncul:

```bash
curl -i https://<domain>.trycloudflare.com/healthz
```

Atur URL webhook provider menjadi:

```text
https://<domain>.trycloudflare.com/webhooks/xendit
https://<domain>.trycloudflare.com/webhooks/midtrans
```

## Troubleshooting Cepat

- `bind: operation not permitted`
  - Biasanya dari batasan environment. Coba jalankan di terminal lokal lain atau gunakan port berbeda.
- `connection refused` di `localhost:8080`
  - Pastikan daemon masih aktif di session yang sama dan belum crash saat dipanggil.
- `502 Bad Gateway` dari URL tunnel
  - Pastikan daemon lokal tetap jalan dan tunnel tetap connected ke `--url` yang benar.
- Gagal resolve domain `trycloudflare.com`
  - Bisa jadi environment memiliki pembatasan DNS/network. Coba perangkat/jaringan lain untuk validasi.

## Migration

Skema SQLite awal ada di:

```text
migrations/0001_initial.sql
```

Migration ini mencakup:

- providers
- provider accounts
- payment intents
- payment attempts
- webhook events
- webhook forwarding targets
- webhook forwarding attempts
- refunds
- audit logs

## Catatan Implementasi Berikutnya

- Tambahkan driver SQLite untuk Go.
- Implementasikan storage SQLite berdasarkan migration awal.
- Hubungkan CLI onboarding ke storage SQLite.
- Implementasikan adapter Midtrans dan Xendit sesuai dokumentasi resmi provider.
