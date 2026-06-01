---
title: Provider setup
description: Onboard Xendit, Midtrans, and DOKU today, then track Flip Business and Duitku as provider roadmap work.
lang: en
order: 2
---

Rute Bayar keeps provider behavior inside modular adapters. The current stable provider set includes Xendit, Midtrans, and DOKU Checkout.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard midtrans --environment sandbox
rutebayar onboard doku --client-id "$DOKU_CLIENT_ID" --secret-key "$DOKU_SECRET_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
rutebayar provider test doku
```

Provider roadmap:

- Flip Business
- Duitku

Each provider should preserve raw outbound request JSON, raw provider response JSON, inbound webhook JSON, and inbound webhook headers.
