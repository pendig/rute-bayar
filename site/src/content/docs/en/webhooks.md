---
title: Webhook daemon
description: Run the webhook daemon, register provider callback URLs, and replay stored events.
lang: en
order: 3
---

The daemon receives provider callbacks and stores the original payloads for debugging and replay.

```bash
rutebayar webhook serve --addr :8080
curl http://127.0.0.1:8080/healthz
```

Provider callback URLs:

```text
https://<public-domain>/webhooks/xendit
https://<public-domain>/webhooks/midtrans
```

When credentials are configured, the daemon verifies signatures or callback tokens according to each provider adapter.
