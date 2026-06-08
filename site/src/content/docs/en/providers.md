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

## API mode (experimental)

`rutebayar api <provider>` lets you call provider official endpoints directly from the CLI.

Use cases:

- Debug integration when adapter coverage is incomplete.
- Validate provider response behavior before adding provider-specific adapter features.

Quick mapping:

| Provider | Alias | Method | Path |
| - | - | - | - |
| Midtrans | auth-test / auth / ping | GET | /v2/rute-bayar-auth-test/status |
| Midtrans | status / check-status | GET | /v2/{order_id}/status |
| Midtrans | charge / create | POST | /v2/charge |
| Xendit | auth-balance / balance | GET | /balance |
| Xendit | session-create / create | POST | /sessions |
| Xendit | session-status / status | GET | /sessions/{session_id} |
| DOKU | checkout | POST | /checkout/v1/payment |
| DOKU | order-status / status | GET | /orders/v1/status/{invoice_number_or_request_id} |
| iPaymu | payment-channels / channels | GET | /api/v2/payment-channels |
| iPaymu | transaction | POST | /api/v2/transaction |

Examples:

```bash
rutebayar api midtrans --operation status --path-param order_id=rb-demo-001
rutebayar api xendit --operation auth-balance
rutebayar api doku --operation order-status --path-param invoice_number_or_request_id=INV-001
rutebayar api ipaymu --operation payment-channels --method GET
```

Notes:

- If `--operation` has no match, call manually using `--path`.
- This mode complements, not replaces, `pay create/status`.

Provider roadmap:

- Flip Business
- Duitku

Each provider should preserve raw outbound request JSON, raw provider response JSON, inbound webhook JSON, and inbound webhook headers.
