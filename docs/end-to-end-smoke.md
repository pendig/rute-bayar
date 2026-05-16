# End-to-End Smoke Test

Dokumen ini menjadi checklist minimal sebelum release atau sebelum merge perubahan besar.

## Smoke Lokal Otomatis

Jalankan:

```bash
./scripts/smoke-local.sh
```

Script ini melakukan:

- build binary lokal
- migrasi SQLite ke database temporary
- onboard credential Xendit dummy khusus test lokal
- start daemon webhook
- start receiver forwarding lokal
- kirim simulasi webhook Xendit ke daemon
- verifikasi forwarding attempt sukses lewat CLI diagnostics

Port default:

- daemon: `127.0.0.1:18080`
- receiver forwarding: `127.0.0.1:18081`

Override jika port bentrok:

```bash
RUTE_BAYAR_SMOKE_ADDR=127.0.0.1:19080 \
RUTE_BAYAR_SMOKE_FORWARD_ADDR=127.0.0.1:19081 \
./scripts/smoke-local.sh
```

## Smoke Manual Provider Sandbox

Gunakan checklist ini untuk validasi sandbox dengan credential asli yang disimpan di environment lokal, bukan di repo.
Sebelum menjalankan checklist ini, pastikan credential yang pernah dipakai di luar secret store sudah di-rotate.

### Runner Sandbox

Runner ini menjalankan bagian yang bisa dibuat otomatis dari checklist sandbox:

```bash
./scripts/e2e-sandbox.sh
```

Credential dibaca dari environment:

```bash
export XENDIT_SECRET_KEY="..."
export XENDIT_WEBHOOK_TOKEN="..." # optional
export MIDTRANS_MERCHANT_ID="..."
export MIDTRANS_CLIENT_KEY="..."
export MIDTRANS_SERVER_KEY="..."
```

Kontrol provider:

```bash
RUTE_BAYAR_E2E_XENDIT=1 RUTE_BAYAR_E2E_MIDTRANS=0 ./scripts/e2e-sandbox.sh
RUTE_BAYAR_E2E_XENDIT=0 RUTE_BAYAR_E2E_MIDTRANS=1 ./scripts/e2e-sandbox.sh
```

Runner Xendit mengisi customer sandbox default supaya payload Payment Session valid. Nilai ini bisa dioverride jika perlu:

```bash
RUTE_BAYAR_E2E_XENDIT_CUSTOMER_NAME="Rute Bayar Tester" \
RUTE_BAYAR_E2E_XENDIT_CUSTOMER_EMAIL="tester@example.test" \
./scripts/e2e-sandbox.sh
```

Refund membutuhkan transaksi sandbox yang sudah paid/settled/refundable. Untuk menjalankan refund terhadap transaksi yang sudah siap:

```bash
RUTE_BAYAR_E2E_XENDIT_REFUND_REFERENCE="rb-paid-xendit-001" \
RUTE_BAYAR_E2E_XENDIT_REFUND_PROVIDER_REFERENCE="ps_xxx_or_pr_xxx" \
./scripts/e2e-sandbox.sh
```

```bash
RUTE_BAYAR_E2E_MIDTRANS_REFUND_REFERENCE="rb-paid-midtrans-001" \
RUTE_BAYAR_E2E_MIDTRANS_REFUND_PROVIDER_REFERENCE="order-or-transaction-id" \
./scripts/e2e-sandbox.sh
```

Midtrans refund real membutuhkan metode yang refundable menurut Midtrans, seperti credit card/e-wallet/QRIS, dan status transaksi harus `settlement`. Untuk credit card Core API, buat `token_id` terlebih dahulu melalui Midtrans Get Token API atau MidtransNew3ds JS, lalu jalankan:

```bash
RUTE_BAYAR_E2E_XENDIT=0 \
RUTE_BAYAR_E2E_MIDTRANS=1 \
RUTE_BAYAR_E2E_MIDTRANS_METHOD=card \
RUTE_BAYAR_E2E_MIDTRANS_CARD_TOKEN="<token_id>" \
./scripts/e2e-sandbox.sh
```

Jika response menghasilkan `redirect_url`, buka URL tersebut dan selesaikan 3DS sandbox. Test card Midtrans sandbox yang umum:

```text
Card Number: 4811111111111114
CVV: 123
Exp Month: 02
Exp Year: tahun future
OTP/3DS: 112233
Bank One Time Token: 12345678
```

Untuk menghindari halaman 3DS mentah berhenti di "Card is authenticated", buka helper lokal ini:

```bash
open "docs/tools/midtrans-3ds.html?client_key=$MIDTRANS_CLIENT_KEY&redirect_url=<urlencoded_redirect_url>"
```

Atau buka `docs/tools/midtrans-3ds.html` di browser, isi Client Key dan `redirect_url`, lalu klik **Start 3DS Authentication**. Helper ini memakai `MidtransNew3ds.redirect()` sesuai rekomendasi Core API Midtrans.

Alternatif refundable yang lebih sederhana dari 3DS card adalah dynamic QRIS. Runner akan menampilkan QR code image URL sebagai `redirect_url`:

```bash
RUTE_BAYAR_E2E_XENDIT=0 \
RUTE_BAYAR_E2E_MIDTRANS=1 \
RUTE_BAYAR_E2E_MIDTRANS_METHOD=qris \
RUTE_BAYAR_E2E_MIDTRANS_QRIS_ACQUIRER=gopay \
./scripts/e2e-sandbox.sh
```

Untuk menyelesaikan sandbox QRIS, copy QR code image URL dari `redirect_url` lalu input ke Midtrans QRIS Simulator:

```text
https://simulator.sandbox.midtrans.com/v2/qris/index
```

### 1. Setup

```bash
export RUTE_BAYAR_ENV=sandbox
export RUTE_BAYAR_DB_PATH=./rute-bayar.sqlite3
rute-bayar db migrate
```

### 2. Onboard Provider

```bash
rute-bayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
rute-bayar onboard midtrans \
  --merchant-id "$MIDTRANS_MERCHANT_ID" \
  --client-key "$MIDTRANS_CLIENT_KEY" \
  --server-key "$MIDTRANS_SERVER_KEY" \
  --environment sandbox
```

### 3. Provider Auth Test

```bash
rute-bayar provider test xendit
rute-bayar provider test midtrans
```

### 4. Payment Create

```bash
rute-bayar pay create \
  --provider xendit \
  --method payment_link \
  --reference rb-smoke-xendit-001 \
  --amount 15000
```

```bash
rute-bayar pay create \
  --provider midtrans \
  --method bank_transfer \
  --bank bca \
  --reference rb-smoke-midtrans-001 \
  --amount 15000
```

Jika ingin mengarahkan notifikasi Midtrans per transaksi ke daemon publik sementara, gunakan `--notification-url`.
Flag ini mengirim header resmi Midtrans `X-Override-Notification` pada request charge:

```bash
rute-bayar pay create \
  --provider midtrans \
  --method qris \
  --bank gopay \
  --reference rb-smoke-midtrans-qris-001 \
  --amount 15000 \
  --notification-url https://<public-domain>/webhooks/midtrans
```

### 5. Payment Status

```bash
rute-bayar pay status --provider xendit --reference rb-smoke-xendit-001
rute-bayar pay status --provider midtrans --reference rb-smoke-midtrans-001
```

### 6. Webhook Daemon

```bash
rute-bayar webhook serve --addr :8080 --environment sandbox
curl -i http://localhost:8080/healthz
```

Provider webhook URL:

```text
https://<public-domain>/webhooks/xendit
https://<public-domain>/webhooks/midtrans
```

Untuk Midtrans, URL di atas juga bisa dioverride per transaksi lewat `pay create --notification-url`.
Untuk Xendit, gunakan pengaturan webhook/callback di dashboard sandbox Xendit.

Untuk domain sementara:

```bash
wrangler tunnel quick-start http://localhost:8080
```

### 7. Forwarding

```bash
rute-bayar webhook forward add \
  --provider xendit \
  --name smoke-forward \
  --url https://example.com/webhooks/rute-bayar \
  --event-filter event=payment_session.created

rute-bayar webhook forward attempts list --provider xendit --limit 20
```

### 8. Replay dan Diagnostics

Ambil event ID dari database atau log, lalu:

```bash
rute-bayar webhook replay --provider xendit --event-id <webhook_event_id>
rute-bayar webhook forward attempts list --status failed
rute-bayar webhook forward attempts show <attempt_id>
rute-bayar webhook forward attempts retry <attempt_id>
```

### 9. Refund dan Reconcile

```bash
rute-bayar pay refund --provider xendit --reference rb-smoke-xendit-001 --amount 15000
rute-bayar reconcile --provider xendit --reference rb-smoke-xendit-001
```

Untuk Midtrans, pastikan transaksi sudah berada pada status yang boleh direfund menurut provider sebelum menjalankan:

```bash
rute-bayar pay refund --provider midtrans --reference rb-smoke-midtrans-001 --amount 15000
rute-bayar reconcile --provider midtrans --reference rb-smoke-midtrans-001
```

## Acceptance

Smoke dianggap cukup untuk alpha/stable candidate jika:

- `provider test` sukses untuk provider yang diuji
- `pay create` menghasilkan provider reference
- `pay status` bisa membaca status provider
- webhook diterima daemon dengan `202 Accepted`
- forwarding attempt tercatat
- failed forwarding dapat dilihat lewat diagnostics
- replay/retry dapat dijalankan manual
- refund/reconcile tidak merusak status lokal
