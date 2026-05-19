---
title: Setup provider
description: Onboard Xendit dan Midtrans hari ini, lalu ikuti roadmap Doku, Flip Business, dan Duitku.
lang: id
order: 2
---

Rute Bayar menjaga perilaku provider tetap berada di adapter modular. Provider stabil saat ini dimulai dari Xendit dan Midtrans.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard midtrans --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
```

Roadmap provider:

- Doku
- Flip Business
- Duitku

Setiap provider harus menyimpan raw outbound request JSON, raw provider response JSON, inbound webhook JSON, dan inbound webhook headers.
