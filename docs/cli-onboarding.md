# CLI Onboarding

## Tujuan

CLI onboarding dibuat supaya user bisa setup provider dengan cepat tanpa perlu konfigurasi manual yang tersebar di banyak tempat.

## Prinsip

- Satu alur onboarding yang konsisten untuk semua provider.
- Validasi credential langsung saat setup.
- Simpan konfigurasi ke SQLite.
- Tampilkan capability provider supaya user tahu fitur apa yang tersedia.
- Forwarding target bisa diatur dari CLI.

## Command Flow

Contoh alur:

1. `rute-bayar onboard`
2. Pilih provider
3. Pilih environment `sandbox` atau `production`
4. Masukkan credential
5. Jalankan test connection
6. Simpan configuration
7. Tampilkan ringkasan capability

## Perintah CLI yang Disarankan

- `rute-bayar onboard`
- `rute-bayar provider list`
- `rute-bayar provider test`
- `rute-bayar config show`
- `rute-bayar config set`
- `rute-bayar pay create`
- `rute-bayar pay status`
- `rute-bayar pay refund`
- `rute-bayar webhook serve`
- `rute-bayar webhook replay`
- `rute-bayar webhook forward list`
- `rute-bayar webhook forward add`
- `rute-bayar webhook forward update`
- `rute-bayar webhook forward remove`
- `rute-bayar reconcile`

## Onboarding Provider

### Midtrans

CLI harus meminta:

- server key
- client key jika dibutuhkan
- environment
- webhook secret atau mekanisme verifikasi yang dipakai
- default callback URL

Setelah itu CLI harus:

- test status endpoint
- test signature verification flow
- simpan provider account

### Xendit

CLI harus meminta:

- secret API key
- webhook token / secret
- environment
- callback URL

Setelah itu CLI harus:

- test auth ke API
- test webhook verification helper
- simpan provider account

## UX yang Disarankan

- Gunakan wizard interaktif.
- Tampilkan nilai default yang aman.
- Jangan minta terlalu banyak input sekaligus.
- Setelah onboarding, tunjukkan langkah berikutnya yang paling relevan.

## Forwarding Setup

CLI harus menyediakan alur untuk:

- menambahkan target forwarding
- memilih provider atau event yang diforward
- mengatur retry policy default
- mengubah retry policy via CLI
- menyalakan atau mematikan forwarding per target

Forwarding default harus pass-through, artinya payload diteruskan apa adanya dari provider.

## Output Onboarding

Setelah sukses, CLI sebaiknya menampilkan:

- provider yang aktif
- environment yang dipilih
- endpoint webhook yang harus didaftarkan ke provider
- fitur yang didukung
- contoh perintah create payment pertama
- status forwarding yang aktif jika ada
