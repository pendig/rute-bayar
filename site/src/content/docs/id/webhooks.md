---
title: Webhook daemon
description: Jalankan daemon webhook, daftarkan callback URL provider, dan replay event yang tersimpan.
lang: id
order: 3
---

Daemon menerima callback provider dan menyimpan payload asli untuk debugging dan replay.

```bash
rutebayar webhook serve --addr :8080
curl http://127.0.0.1:8080/healthz
```

Callback URL provider:

```text
https://<public-domain>/webhooks/xendit
https://<public-domain>/webhooks/midtrans
https://<public-domain>/webhooks/doku
```

Saat credential tersedia, daemon memverifikasi signature atau callback token sesuai adapter masing-masing provider.
