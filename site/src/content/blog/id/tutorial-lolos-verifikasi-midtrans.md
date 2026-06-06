---
title: "Tutorial lolos verifikasi Midtrans"
description: "Checklist aktivasi Midtrans 2026: dokumen legal, bukti bisnis online, dan hal teknis yang perlu rapi sebelum submit."
lang: id
pubDate: 2026-06-06
---

Verifikasi Midtrans bukan sekadar mengisi form. Tim review perlu melihat bahwa bisnisnya nyata, dokumennya sesuai, dan kanal jualannya bisa diperiksa publik. Panduan ini dirangkum dari dokumentasi resmi Midtrans yang masih menjadi rujukan saat artikel ini ditulis pada 6 Juni 2026.

## Ringkasan cepat

Midtrans meminta merchant mendaftar lewat dashboard, memverifikasi email, lalu menyelesaikan proses aktivasi/passport. Di dokumentasi pendaftaran, Midtrans juga menegaskan bahwa URL website, aplikasi, marketplace, atau social media harus dapat diakses publik, menampilkan produk, dan memperlihatkan harga.

## Checklist sebelum submit

1. Siapkan email bisnis dan nomor telepon yang belum pernah dipakai di akun Midtrans, GoBiz, atau GoFood lain.
2. Pastikan website, aplikasi, marketplace, atau social media aktif dan terbuka tanpa login.
3. Tampilkan produk atau layanan dengan deskripsi yang jelas.
4. Tampilkan harga dalam Rupiah.
5. Pastikan kategori bisnis yang kamu pilih cocok dengan isi website atau social media.
6. Siapkan dokumen legal sesuai tipe usaha.

Untuk usaha perorangan, dokumen inti yang disebut Midtrans adalah KTP pemilik dan NPWP. Untuk badan usaha seperti PT, CV, atau PMA, siapkan akta perusahaan terbaru, SK Kemenkumham, KTP atau paspor direktur, NPWP direktur, NPWP perusahaan, NIB/SIUP/TDP, dan izin usaha lain sesuai aktivitas bisnis.

## Cara menghindari bolak-balik revisi

Masalah yang sering membuat aktivasi tertahan biasanya bukan integrasi API, tetapi bukti bisnis yang belum cukup jelas. Sebelum submit, buka URL bisnis dari browser incognito dan cek apakah reviewer bisa langsung memahami apa yang dijual, berapa harganya, dan bagaimana customer membeli.

Kalau bisnis masih memakai Instagram atau marketplace, rapikan bio, highlight, katalog, contoh produk, harga, dan kontak. Jangan submit akun kosong, private, atau halaman yang isinya belum match dengan kategori bisnis.

## Setelah akun aktif

Setelah produksi aktif, pisahkan konfigurasi sandbox dan production. Untuk integrasi Rute Bayar, simpan credential Midtrans di environment atau database lokal, lalu validasi dengan command provider sebelum menerima transaksi nyata.

```bash
rutebayar onboard midtrans --environment production
rutebayar provider list
```

## Sumber resmi

- [Cara mendaftar sebagai merchant Midtrans](https://docs.midtrans.com/docs/bagaimana-cara-mendaftar-menjadi-merchant-midtrans)
- [Kriteria website atau aplikasi untuk registrasi Midtrans](https://docs.midtrans.com/docs/what-are-the-website-or-application-criterias-for-registering-a-midtrans-account)
- [Dokumen legal untuk registrasi Midtrans](https://docs.midtrans.com/docs/what-are-the-legal-documents-required-for-midtrans-account-registration)
