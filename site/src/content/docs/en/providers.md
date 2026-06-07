---
title: Provider setup
description: Onboard Xendit, Midtrans, DOKU, and iPaymu today, then track Flip Business and Duitku as provider roadmap work.
lang: en
order: 2
---

Rute Bayar keeps provider behavior inside modular adapters. The current stable provider set includes Xendit, Midtrans, DOKU Checkout, and iPaymu.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard midtrans --environment sandbox
rutebayar onboard doku --client-id "$DOKU_CLIENT_ID" --secret-key "$DOKU_SECRET_KEY" --environment sandbox
rutebayar onboard ipaymu --va "$IPAYMU_VA" --api-key "$IPAYMU_API_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
rutebayar provider test doku
rutebayar provider test ipaymu
```

## Refund capability

Refunds are provider/channel capability-specific, not a universal feature across every provider.

- Xendit and Midtrans have initial refund flows.
- DOKU refunds are not enabled yet because they require Refund API/disbursement setup.
- iPaymu refunds are not available yet because the public iPaymu API v2 does not expose an official verified refund endpoint. Treat it as unsupported until iPaymu provides an official endpoint and payload.

Provider roadmap:

- Flip Business
- Duitku

Each provider should preserve raw outbound request JSON, raw provider response JSON, inbound webhook JSON, and inbound webhook headers.
