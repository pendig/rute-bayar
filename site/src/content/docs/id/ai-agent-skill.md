---
title: Panduan skill AI Agent
description: Ajarkan AI Agent membuat invoice, mengecek status, dan reconcile payment lewat rutebayar.
lang: id
order: 5
---

Rute Bayar bisa menjadi batas command yang hati-hati untuk penagihan AI Agent. Agent tidak perlu logic provider spesifik; agent hanya perlu beberapa command kecil.

```bash
rutebayar pay create --provider xendit --reference agent-run-1001 --amount 25000
rutebayar pay status --provider xendit --reference agent-run-1001
rutebayar reconcile --provider xendit --reference agent-run-1001
```

Perilaku agent yang direkomendasikan:

- Gunakan reference unik untuk setiap run berbayar atau action produk.
- Simpan payment URL dan provider reference yang dikembalikan.
- Utamakan status webhook yang sudah diverifikasi untuk keputusan final.
- Jalankan reconcile sebelum fulfillment jika eksekusi sempat terputus.
