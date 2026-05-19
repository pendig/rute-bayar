---
title: Kenapa Rute Bayar dimulai dari CLI
description: CLI adalah kontrak kecil yang stabil antara product code, operasi, dan payment provider.
lang: id
pubDate: 2026-05-19
---

Pekerjaan payment butuh permukaan yang mudah diaudit. CLI memberi Rute Bayar batas yang sempit dan eksplisit untuk membuat payment, cek status, menjalankan command operasional, dan membantu AI Agent bekerja tanpa menanam logic provider.

Daemon menangani trafik webhook. CLI menangani workflow manusia dan automation. Keduanya membuat versi pertama tetap kecil untuk dipahami, tapi cukup serius untuk dioperasikan.
