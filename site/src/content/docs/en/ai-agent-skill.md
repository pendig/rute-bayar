---
title: AI Agent skill guide
description: Teach an AI Agent how to create invoices, check status, and reconcile payment state through rutebayar.
lang: en
order: 5
---

Rute Bayar can act as a careful command boundary for AI Agent billing. The agent does not need provider-specific logic; it only needs a small set of commands.

```bash
rutebayar pay create --provider xendit --reference agent-run-1001 --amount 25000
rutebayar pay status --provider xendit --reference agent-run-1001
rutebayar reconcile --provider xendit --reference agent-run-1001
```

Recommended agent behavior:

- Use a unique reference for every paid run or product action.
- Store the returned payment URL and provider reference.
- Prefer verified webhook state for final decisions.
- Run reconcile before fulfillment if execution was interrupted.
