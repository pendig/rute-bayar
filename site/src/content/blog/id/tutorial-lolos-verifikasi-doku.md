---
title: "Tutorial lolos verifikasi DOKU"
description: "Checklist aktivasi DOKU 2026 untuk Business Account, dokumen legal, bukti bisnis, dan batas akun belum terverifikasi."
lang: id
pubDate: 2026-06-06
---

Verifikasi DOKU Business Account menentukan apakah settlement dan seluruh fitur dashboard bisa dipakai penuh. Panduan ini memakai dokumentasi resmi DOKU yang terakhir terlihat diperbarui sekitar 2026 dan dicek pada 6 Juni 2026.

## Ringkasan cepat

DOKU meminta merchant mengaktifkan Business Account, mengisi data bisnis, mengunggah dokumen legal, lalu menunggu proses verifikasi. Dokumentasi DOKU menyebut proses verifikasi dapat memakan waktu hingga 48 jam setelah semua dokumen berhasil diunggah. Untuk merchant personal dan corporate, pembayaran tertentu bisa mulai diterima setelah aktivasi Business Account, tetapi settlement dana baru diproses setelah akun berhasil diverifikasi.

## Checklist sebelum submit

1. Tentukan tipe Business Account: Personal, Corporate, atau International.
2. Isi data bisnis, data representative, dan brand dengan konsisten.
3. Upload bukti bisnis yang benar-benar menunjukkan lokasi, aktivitas, atau produk.
4. Siapkan dokumen legal sesuai tipe akun.
5. Pastikan dokumen terbaca jelas, tidak blur, tidak disensor, belum kedaluwarsa, dan dimiliki oleh perusahaan.
6. Gunakan format PDF, JPG, JPEG, atau PNG dengan ukuran maksimal 15 MB.
7. Cek status dan catatan revisi dari dashboard DOKU.

Untuk Corporate, dokumentasi DOKU mencantumkan dokumen seperti NIB, akta pendirian dan perubahan perusahaan, SK Kemenkumham, foto bukti bisnis, NPWP, dan KTP direktur. Beberapa lini bisnis memerlukan dokumen tambahan, misalnya izin OJK untuk peer-to-peer lending atau lisensi Bank Indonesia untuk PSP/PJSP.

## Pahami batas akun belum terverifikasi

DOKU membatasi akun yang belum terverifikasi. Dokumentasi DOKU menyebut Personal Merchant dapat menerima hingga 5 transaksi dengan total volume maksimal IDR 1.000.000, sedangkan Corporate Merchant hingga 5 transaksi dengan total volume maksimal IDR 10.000.000. International Merchant harus menyelesaikan verifikasi sebelum dapat menerima pembayaran.

## Cara menghindari status Under Review berkepanjangan

Jika status tetap Under Review, cek menu Settings di DOKU Dashboard. Pada Business Info dan Documents, DOKU dapat menampilkan banner kuning untuk data yang masih diverifikasi atau banner merah untuk data yang ditolak dan perlu diperbaiki. Jangan submit ulang sebagian; perbaiki semua catatan agar review tidak diulang dari awal.

Untuk integrasi Rute Bayar, aktifkan sandbox lebih dulu, lalu pindahkan production credential setelah Business Account dan channel payment yang dibutuhkan sudah siap.

```bash
rutebayar onboard doku --environment sandbox
rutebayar onboard doku --environment production
```

## Sumber resmi

- [Activate Business - DOKU Docs](https://docs.doku.com/get-started/activate-business)
- [Business Account - DOKU Docs](https://docs.doku.com/get-started/activate-business/business-account)
- [Requirements and Limitations - DOKU Docs](https://docs.doku.com/accept-payments/payment-methods/requirements-and-limitations)
