---
title: Setup provider
description: Onboard Xendit, Midtrans, DOKU, dan iPaymu hari ini, lalu ikuti roadmap Flip Business dan Duitku.
lang: id
order: 2
---

Rute Bayar menjaga perilaku provider tetap berada di adapter modular. Provider stabil saat ini mencakup Xendit, Midtrans, DOKU Checkout, dan iPaymu.

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

## Capability refund

Refund bersifat capability-specific per provider/channel, bukan fitur universal semua provider.

- Xendit dan Midtrans memiliki flow refund awal.
- DOKU refund belum diaktifkan karena membutuhkan setup Refund API/disbursement.
- iPaymu refund belum tersedia karena API publik iPaymu v2 belum mengekspos endpoint refund resmi/terverifikasi. Perlakukan sebagai unsupported sampai iPaymu menyediakan endpoint dan payload resmi.

## API mode (eksperimental)

`rutebayar api <provider>` memungkinkan kamu memanggil endpoint resmi provider langsung dari CLI.

Kasus penggunaan:

- Debug integrasi saat adapter internal belum menutupi endpoint tertentu.
- Membandingkan behavior API provider sebelum menambahkan fitur khusus di adapter.

Mapping cepat:

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

Contoh:

```bash
rutebayar api midtrans --operation status --path-param order_id=rb-demo-001
rutebayar api xendit --operation auth-balance
rutebayar api doku --operation order-status --path-param invoice_number_or_request_id=INV-001
rutebayar api ipaymu --operation payment-channels --method GET
```

Catatan:

- Jika `--operation` tidak cocok, endpoint bisa dipanggil manual via `--path`.
- Jalur ini tidak menggantikan command `pay create/status`.

Roadmap provider:

- Flip Business
- Duitku

Setiap provider harus menyimpan raw outbound request JSON, raw provider response JSON, inbound webhook JSON, dan inbound webhook headers.
