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

1. `rutebayar onboard`
2. Pilih provider
3. Pilih environment `sandbox` atau `production`
4. Masukkan credential
5. Jalankan test connection
6. Simpan configuration
7. Tampilkan ringkasan capability

## Perintah CLI yang Disarankan

- `rutebayar onboard`
- `rutebayar onboard xendit --secret-key <key> --environment sandbox`
- `rutebayar onboard midtrans --merchant-id <id> --client-key <key> --server-key <key> --environment sandbox`
- `rutebayar onboard doku --client-id <id> --secret-key <key> --environment sandbox`
- `rutebayar provider list`
- `rutebayar provider accounts`
- `rutebayar provider test midtrans`
- `rutebayar provider test xendit`
- `rutebayar provider test doku`
- `rutebayar config show`
- `rutebayar config set`
- `rutebayar pay create`
- `rutebayar pay status`
- `rutebayar pay refund`
- `rutebayar webhook serve`
- `rutebayar webhook replay`
- `rutebayar webhook forward list`
- `rutebayar webhook forward add`
- `rutebayar webhook forward update`
- `rutebayar webhook forward remove`
- `rutebayar reconcile`

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

Command awal yang sudah ditargetkan:

```bash
rutebayar onboard midtrans --merchant-id "$MIDTRANS_MERCHANT_ID" --client-key "$MIDTRANS_CLIENT_KEY" --server-key "$MIDTRANS_SERVER_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test midtrans
```

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

Command awal yang sudah ditargetkan:

```bash
rutebayar onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test xendit
```

### DOKU

CLI harus meminta:

- client ID
- secret key
- environment
- webhook target path untuk signature verification

Setelah itu CLI harus:

- test signed request ke Check Status API
- simpan provider account
- pakai `/webhooks/doku` sebagai default webhook path

Command awal yang sudah tersedia:

```bash
rutebayar onboard doku --client-id "$DOKU_CLIENT_ID" --secret-key "$DOKU_SECRET_KEY" --environment sandbox
rutebayar provider accounts
rutebayar provider test doku
```

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
