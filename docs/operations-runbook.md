# Operations Runbook

Dokumen ini membantu tim operasi menjalankan, mengecek, dan merecovery alur webhook di environment lokal/non-produksi.

## Smoke Test Cepat

1. Migrasi database:

```bash
rute-bayar db migrate
```

2. Aktifkan daemon:

```bash
rute-bayar webhook serve --addr :8080 --environment sandbox
```

3. Cek kesehatan service:

```bash
curl -i http://localhost:8080/healthz
```

Respon sukses:

```json
{"status":"ok"}
```

4. Simulasi webhook lokal:

```bash
curl -X POST http://localhost:8080/webhooks/xendit \
  -H 'Content-Type: application/json' \
  -d '{"event":"payment_session.created","status":"ACTIVE","reference_id":"rb-1001"}'
```

## Webhook Replay

Setiap webhook yang masuk disimpan dengan `id` di tabel `webhook_events`.
Jika forwarding perlu dicoba ulang atau diagnosa, jalankan:

```bash
rute-bayar webhook replay --event-id <id> --db ./rute-bayar.sqlite3
```

Contoh:

```bash
rute-bayar webhook replay --event-id webhook_1715001
```

Optional: validasi provider mismatch.

```bash
rute-bayar webhook replay --provider midtrans --event-id webhook_1715001
```

## Cloudflare Tunnel untuk Uji Callback Public

Jika callback provider butuh URL public sementara:

```bash
wrangler tunnel quick-start http://localhost:8080
```

Verifikasi endpoint publik:

```bash
curl -i https://<domain>.trycloudflare.com/healthz
```

Setelah itu daftar webhook provider ke:

```text
https://<domain>.trycloudflare.com/webhooks/xendit
https://<domain>.trycloudflare.com/webhooks/midtrans
```

## Log dan Diagnostik

- Raw inbound/outbound event tersimpan di tabel:
  - `webhook_events.payload_json`
  - `webhook_events.headers_json`
  - `webhook_forwarding_attempts.request_json`
  - `webhook_forwarding_attempts.response_json`

- Untuk lihat event terbaru:

```bash
sqlite3 ./rute-bayar.sqlite3 \
  "SELECT id, provider_id, provider_event_id, event_type, processing_status, signature_valid FROM webhook_events ORDER BY received_at DESC LIMIT 20;"
```

- Untuk lihat forwarding attempt terakhir:

```bash
sqlite3 ./rute-bayar.sqlite3 \
  "SELECT id, webhook_event_id, forwarding_target_id, status, attempt_no, created_at FROM webhook_forwarding_attempts ORDER BY created_at DESC LIMIT 20;"
```

- Alternatif lewat CLI:

```bash
rute-bayar webhook forward attempts list --limit 20
rute-bayar webhook forward attempts list --status failed
rute-bayar webhook forward attempts show <attempt_id>
rute-bayar webhook forward attempts retry <attempt_id>
```

## Troubleshooting

### Daemon tidak menerima request

- Pastikan `webhook serve` masih hidup.
- Pastikan `--addr` sesuai dengan port yang aktif.
- `connection refused` biasanya berarti daemon belum berjalan atau bound ke IP/port lain.

### Signature/token gagal

- Pastikan credential provider untuk environment yang sama dengan webhook endpoint yang di-call.
- Pastikan header token (`X-Callback-Token`) tidak berubah jika onboarding menyimpannya.

### Replay tidak terkirim

- Pastikan event ID valid dan header/body masih tersimpan.
- Pastikan target forwarding masih `enabled`.
- Pastikan `event_filter` target tidak mem-filter event tersebut.
