---
title: Penagihan AI Agent dengan payment reference
description: Model praktis agar agent bisa membuat invoice dan memverifikasi status payment lewat batas command yang sempit.
lang: id
pubDate: 2026-05-19
---

Penagihan AI Agent lebih mudah dipikirkan ketika setiap payment punya reference yang stabil. Run ID, tenant ID, atau action produk bisa menjadi payment reference, sementara Rute Bayar menangani request shape dan status mapping khusus provider.

Agent bisa membuat invoice, menunggu user membayar, memverifikasi status, dan melanjutkan pekerjaan hanya setelah reconcile mengonfirmasi state.
