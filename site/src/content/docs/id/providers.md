---
title: Setup provider
description: Onboard Xendit, Midtrans, dan DOKU hari ini, lalu ikuti roadmap Flip Business dan Duitku.
lang: id
order: 2
---

Rute Bayar menjaga perilaku provider tetap berada di adapter modular. Provider stabil saat ini mencakup Xendit, Midtrans, dan DOKU Checkout.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard midtrans --environment sandbox
rutebayar onboard doku --client-id "$DOKU_CLIENT_ID" --secret-key "$DOKU_SECRET_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
rutebayar provider test doku
```

Roadmap provider:

- Flip Business
- Duitku

Setiap provider harus menyimpan raw outbound request JSON, raw provider response JSON, inbound webhook JSON, dan inbound webhook headers.
