# Implementation Plan

Dokumen ini memecah pekerjaan menuju release pertama Rute Bayar menjadi milestone yang lebih kecil dan bisa dieksekusi bertahap.

## Milestone 1: Core Payment Status

Tujuan:

- membuat `pay status` benar-benar bekerja
- menyatukan model status internal dengan hasil dari provider

Pekerjaan:

- implement `pay status` untuk Midtrans
- implement `pay status` untuk Xendit
- finalisasi mapping status internal
- simpan raw request/response JSON untuk status inquiry
- tambahkan unit test untuk parser respons status

Output:

- user bisa mengecek status payment dari CLI
- status internal konsisten untuk dua provider utama

## Milestone 2: Webhook Verification

Tujuan:

- memastikan daemon tidak menerima webhook palsu
- membuat webhook bisa diproses secara aman dan idempotent

Pekerjaan:

- implement verifikasi signature Midtrans
- implement verifikasi webhook Xendit
- parsing payload provider ke event internal
- simpan inbound payload dan headers mentah
- update payment intent dari webhook
- tambahkan unit test untuk signature verification dan parsing

Output:

- daemon webhook bisa dipakai dengan aman di sandbox maupun production

## Milestone 3: Forwarding Management

Tujuan:

- membuat forwarding webhook bisa dikonfigurasi penuh dari CLI

Pekerjaan:

- implement CRUD target forwarding
- simpan target forwarding di SQLite
- tambah retry policy configurable
- tambah command replay/diagnostic
- tambahkan unit test untuk penyimpanan target dan retry policy

Output:

- user bisa menambah, melihat, mengubah, dan menghapus target forwarding lewat CLI

Rujukan detail: [Forwarding Feature Plan](./forwarding-feature-plan.md).

## Milestone 4: Operational Hardening

Tujuan:

- memastikan project mudah dijalankan dan diuji oleh contributor

Pekerjaan:

- aktifkan `go test ./...` di environment CI
- tambah GitHub Actions untuk test dan formatting
- pastikan `gofmt` dijalankan konsisten
- validasi migrasi SQLite dari kondisi fresh install
- audit repo agar tidak ada secret provider tersimpan

Output:

- setiap kontribusi penting bisa diverifikasi otomatis

## Milestone 5: Release Engineering

Tujuan:

- menyiapkan tag release publik pertama

Pekerjaan:

- tulis changelog untuk `v0.1.0`
- pastikan README install dan usage stabil
- buat release notes singkat
- verifikasi binary build dan install
- tag release pertama

Output:

- project siap dipublikasikan sebagai release awal

## Urutan Rekomendasi

1. Core Payment Status
2. Webhook Verification
3. Forwarding Management
4. Operational Hardening
5. Release Engineering
