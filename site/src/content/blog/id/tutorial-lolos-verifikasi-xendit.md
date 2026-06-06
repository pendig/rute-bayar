---
title: "Tutorial lolos verifikasi Xendit"
description: "Checklist aktivasi Xendit 2026 untuk KYC, dokumen bisnis Indonesia, authorized representative, dan aktivasi instan."
lang: id
pubDate: 2026-06-06
---

Verifikasi Xendit berpusat pada KYC bisnis: siapa pemilik atau perwakilan yang berwenang, apakah dokumen legal lengkap, dan apakah aktivitas bisnis sesuai kebijakan. Panduan ini memakai dokumentasi resmi Xendit dan Help Center Indonesia yang dicek pada 6 Juni 2026.

## Ringkasan cepat

Akun baru Xendit bisa dipakai di Test Mode untuk simulasi. Untuk masuk Live Mode, merchant perlu mengisi data aktivasi, mengunggah dokumen yang diminta, dan melewati proses KYC. Help Center Xendit Indonesia menyebut proses pembuatan sampai aktivasi dapat memakan waktu sekitar satu minggu, sementara artikel aktivasi akun menyebut estimasi validasi 24 jam setelah dokumen lengkap diterima.

## Checklist sebelum submit

1. Buat akun dari dashboard Xendit dan verifikasi email.
2. Lengkapi profil bisnis sesuai bentuk usaha dan negara registrasi.
3. Pastikan authorized representative adalah orang yang berwenang mewakili bisnis.
4. Siapkan identitas yang jelas dan masih berlaku untuk representative.
5. Siapkan bukti alamat jika dokumen identitas tidak memuat alamat.
6. Siapkan bukti otorisasi jika representative tidak tercantum sebagai pihak berwenang di dokumen legal.
7. Pastikan kamera dan browser siap untuk liveness verification.

Untuk Indonesia, Xendit menyediakan halaman dokumen bisnis khusus Indonesia yang diperbarui pada 5 Januari 2026. Karena daftar dokumen dapat berbeda per tipe entity dan produk, gunakan halaman tersebut sebagai rujukan final sebelum submit.

## Aktivasi instan bukan full approval

Xendit memiliki konsep Aktivasi Instan untuk sebagian bisnis Indonesia yang berisiko rendah dan tidak memerlukan lisensi tambahan. Jika eligible, akun dapat menerima transaksi Money-In lebih cepat. Namun fitur ini tidak langsung membuka Money-Out, withdrawal, atau seluruh channel tambahan; Xendit tetap menyelesaikan KYC dokumen terlebih dahulu.

## Cara menghindari ditolak

Pastikan semua data konsisten: nama legal, nama brand, NPWP/NIB, rekening settlement, alamat, dan website bisnis jangan saling bertabrakan. Jika Xendit mengirim email permintaan update informasi, jawab lewat dashboard dan ubah semua item yang diminta, bukan hanya satu dokumen.

Untuk integrasi Rute Bayar, gunakan Test Mode sampai akun Live benar-benar aktif dan key production tersedia.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard xendit --environment production
```

## Sumber resmi

- [Verifying your account - Xendit Docs](https://docs.xendit.co/docs/verifying-your-account)
- [Authorized representative requirements - Xendit Docs](https://docs.xendit.co/docs/authorized-representative-requirements)
- [Berapa lama proses aktivasi akun Xendit?](https://help.xendit.co/hc/id/articles/4412730514841-Berapa-Lama-Proses-Aktivasi-Akun-Xendit)
- [Apa itu Aktivasi Instan?](https://help.xendit.co/hc/id/articles/4415380128025-Apa-Itu-Aktivasi-Instan)
