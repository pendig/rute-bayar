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

Roadmap provider:

- Flip Business
- Duitku

Setiap provider harus menyimpan raw outbound request JSON, raw provider response JSON, inbound webhook JSON, dan inbound webhook headers.
