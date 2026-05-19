---
title: Forwarding
description: Forward webhook provider sebagai JSON pass-through sambil tetap menyimpan event secara lokal.
lang: id
order: 4
---

Forwarding membuat aplikasi kamu bisa menerima callback provider tanpa harus menjadi endpoint callback utama.

```bash
rutebayar webhook forward add \
  --provider xendit \
  --name orders-api \
  --url https://api.example.com/webhooks/payments
```

Payload yang diforward bersifat pass-through. Rute Bayar tetap menyimpan inbound event dan forwarding attempt agar kegagalan bisa diinspeksi dan direplay.
