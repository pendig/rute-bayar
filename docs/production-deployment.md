# Production Deployment

Dokumen ini menjelaskan cara menjalankan Rute Bayar sebagai daemon webhook production di server/VPS.

Rute Bayar bisa dipakai sebagai payment router untuk aplikasi biasa, product SaaS, dan workflow **AI / AI Agent**. Karena berbasis CLI dan daemon, penagihan dapat dibuat fleksibel dan dinamis: aplikasi atau AI Agent dapat membuat invoice/payment berdasarkan product, usage, tenant, provider, atau aturan bisnis yang berubah tanpa mengikat semua logika pembayaran langsung ke satu payment gateway.

## Target Deployment

Arsitektur minimal production:

```text
Internet
  |
  v
Reverse Proxy / TLS
  |
  v
rute-bayar webhook daemon
  |
  v
SQLite database
```

Komponen:

- `rute-bayar webhook serve`: menerima webhook provider.
- SQLite: menyimpan provider account, payment intent, payment attempt, refund, webhook event, dan forwarding attempt.
- Reverse proxy: terminasi TLS dan expose endpoint public.
- Provider dashboard: Midtrans/Xendit diarahkan ke endpoint webhook public.

## Installation

Install dengan Homebrew:

```bash
brew tap pendig/tap
brew install rute-bayar
rute-bayar version
```

Atau gunakan binary release:

```bash
curl -L -o rute-bayar https://github.com/pendig/rute-bayar/releases/download/v0.1.0/rute-bayar-linux-amd64
chmod +x rute-bayar
sudo mv rute-bayar /usr/local/bin/rute-bayar
```

## Directory Layout

Contoh layout server:

```text
/etc/rute-bayar/rute-bayar.env
/var/lib/rute-bayar/rute-bayar.sqlite3
/var/log/rute-bayar/
```

Permission yang disarankan:

```bash
sudo useradd --system --home /var/lib/rute-bayar --shell /usr/sbin/nologin rute-bayar
sudo mkdir -p /etc/rute-bayar /var/lib/rute-bayar /var/log/rute-bayar
sudo chown -R rute-bayar:rute-bayar /var/lib/rute-bayar /var/log/rute-bayar
sudo chmod 700 /etc/rute-bayar
```

## Environment

Buat file env production:

```bash
sudo tee /etc/rute-bayar/rute-bayar.env >/dev/null <<'EOF'
RUTE_BAYAR_ENV=production
RUTE_BAYAR_DB_PATH=/var/lib/rute-bayar/rute-bayar.sqlite3
RUTE_BAYAR_WEBHOOK_ADDR=127.0.0.1:8080
EOF
```

Jangan commit credential provider. Onboard credential langsung ke SQLite production:

```bash
sudo -u rute-bayar env $(cat /etc/rute-bayar/rute-bayar.env | xargs) \
  rute-bayar db migrate

sudo -u rute-bayar env $(cat /etc/rute-bayar/rute-bayar.env | xargs) \
  rute-bayar onboard xendit \
  --secret-key "$XENDIT_SECRET_KEY" \
  --webhook-token "$XENDIT_WEBHOOK_TOKEN" \
  --environment production

sudo -u rute-bayar env $(cat /etc/rute-bayar/rute-bayar.env | xargs) \
  rute-bayar onboard midtrans \
  --merchant-id "$MIDTRANS_MERCHANT_ID" \
  --client-key "$MIDTRANS_CLIENT_KEY" \
  --server-key "$MIDTRANS_SERVER_KEY" \
  --environment production
```

## systemd Service

Buat unit file:

```bash
sudo tee /etc/systemd/system/rute-bayar.service >/dev/null <<'EOF'
[Unit]
Description=Rute Bayar webhook daemon
After=network-online.target
Wants=network-online.target

[Service]
User=rute-bayar
Group=rute-bayar
EnvironmentFile=/etc/rute-bayar/rute-bayar.env
ExecStart=/usr/local/bin/rute-bayar webhook serve --addr ${RUTE_BAYAR_WEBHOOK_ADDR} --environment ${RUTE_BAYAR_ENV} --db ${RUTE_BAYAR_DB_PATH}
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/rute-bayar /var/log/rute-bayar

[Install]
WantedBy=multi-user.target
EOF
```

Install dan start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rute-bayar
sudo systemctl status rute-bayar
```

Health check lokal:

```bash
curl -i http://127.0.0.1:8080/healthz
```

## Reverse Proxy

Contoh Caddy:

```caddyfile
pay.example.com {
  reverse_proxy 127.0.0.1:8080
}
```

Contoh Nginx:

```nginx
server {
  listen 443 ssl http2;
  server_name pay.example.com;

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}
```

Provider webhook URL:

```text
https://pay.example.com/webhooks/xendit
https://pay.example.com/webhooks/midtrans
```

## Provider Dashboard

### Xendit

- Set Payment Sessions webhook URL ke `/webhooks/xendit`.
- Pastikan callback verification token sama dengan token saat onboarding.
- Xendit Payment Sessions tidak mendukung per-payment webhook URL override; gunakan Dashboard URL global dan fitur forwarding Rute Bayar untuk meneruskan payload ke aplikasi lain.

### Midtrans

- Set payment notification URL ke `/webhooks/midtrans`.
- Untuk skenario tertentu, Midtrans bisa menggunakan `pay create --notification-url` sebagai per-transaction override.

## AI / AI Agent Billing

Rute Bayar cocok sebagai lapisan penagihan untuk AI Agent karena:

- CLI dapat dipanggil oleh agent runner, worker, cron, atau backend service.
- Payment provider bisa dipilih secara dinamis per product, tenant, atau availability.
- Raw JSON inbound/outbound tersimpan untuk audit keputusan billing AI Agent.
- Webhook forwarding dapat mengirim event pembayaran ke orchestration layer AI.
- Product billing bisa berkembang dari one-time payment ke usage-based billing tanpa mengunci integrasi ke satu provider.

Contoh flow:

```text
AI Agent computes billable usage
  |
  v
rute-bayar pay create --provider xendit --reference ai-agent-run-1001 --amount 25000
  |
  v
Provider hosted payment / payment method
  |
  v
Webhook masuk ke Rute Bayar
  |
  v
Rute Bayar reconcile + forward event ke backend product / AI orchestration
```

## SQLite Backup

SQLite production perlu backup reguler:

```bash
sudo -u rute-bayar sqlite3 /var/lib/rute-bayar/rute-bayar.sqlite3 \
  ".backup '/var/lib/rute-bayar/backups/rute-bayar-$(date +%Y%m%d%H%M%S).sqlite3'"
```

Rekomendasi:

- backup sebelum upgrade binary.
- backup harian untuk production traffic.
- simpan backup di storage terpisah.
- uji restore secara berkala.

## Logging and Diagnostics

Log daemon:

```bash
journalctl -u rute-bayar -f
```

Webhook terbaru:

```bash
sqlite3 /var/lib/rute-bayar/rute-bayar.sqlite3 \
  "SELECT id, provider_id, event_type, processing_status, signature_valid, received_at FROM webhook_events ORDER BY received_at DESC LIMIT 20;"
```

Forwarding attempts:

```bash
rute-bayar webhook forward attempts list --db /var/lib/rute-bayar/rute-bayar.sqlite3 --limit 20
rute-bayar webhook forward attempts show <attempt-id> --db /var/lib/rute-bayar/rute-bayar.sqlite3
rute-bayar webhook forward attempts retry <attempt-id> --db /var/lib/rute-bayar/rute-bayar.sqlite3
```

## Upgrade

1. Backup SQLite.
2. Install binary baru.
3. Jalankan migration.
4. Restart service.
5. Cek `/healthz`.
6. Cek log daemon dan webhook event terbaru.

Contoh:

```bash
brew upgrade rute-bayar
sudo -u rute-bayar env $(cat /etc/rute-bayar/rute-bayar.env | xargs) rute-bayar db migrate
sudo systemctl restart rute-bayar
curl -i https://pay.example.com/healthz
```

## Rollback

1. Stop service.
2. Restore binary versi sebelumnya.
3. Restore database dari backup jika migration tidak backward-compatible.
4. Start service.
5. Verifikasi `/healthz` dan webhook endpoint.

## Security Checklist

- Gunakan HTTPS public endpoint.
- Jangan expose SQLite file lewat web server.
- Batasi permission `/etc/rute-bayar` dan `/var/lib/rute-bayar`.
- Rotasi credential provider jika dicurigai bocor.
- Pastikan Xendit callback token dan Midtrans signature verification aktif.
- Monitor event `verification_failed`, `parse_failed`, dan `reconcile_failed`.
- Jangan sensor log jika memang dibutuhkan untuk debugging, tetapi batasi akses log ke operator terpercaya.
