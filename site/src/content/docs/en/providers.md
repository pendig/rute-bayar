---
title: Provider setup
description: Onboard Xendit and Midtrans today, then track Doku, Flip Business, and Duitku as provider roadmap work.
lang: en
order: 2
---

Rute Bayar keeps provider behavior inside modular adapters. The current stable provider set starts with Xendit and Midtrans.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard midtrans --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
```

Provider roadmap:

- Doku
- Flip Business
- Duitku

Each provider should preserve raw outbound request JSON, raw provider response JSON, inbound webhook JSON, and inbound webhook headers.
