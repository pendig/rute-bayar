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
