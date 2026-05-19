---
title: Forwarding
description: Forward provider webhooks as pass-through JSON while keeping events stored locally.
lang: en
order: 4
---

Forwarding lets your application receive provider callbacks without becoming the primary callback endpoint.

```bash
rutebayar webhook forward add \
  --provider xendit \
  --name orders-api \
  --url https://api.example.com/webhooks/payments
```

Forwarded payloads are pass-through. Rute Bayar still stores the inbound event and forwarding attempts so failures can be inspected and replayed.
