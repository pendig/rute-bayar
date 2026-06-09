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
- `rutebayar api <provider> --operation <slug> --method <method> --path <url>` (experimental)
- `rutebayar api <provider> --operation <slug>` (jalankan alias cepat ke endpoint resmi provider)
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

### Catatan API mode (experimental)

Mode ini dipakai untuk memanggil API resmi provider langsung dari CLI.

Sebelum testing endpoint resmi:

- onboarding provider harus sesuai environment target (`sandbox`/`production`)
- kredensial sudah ada di `provider accounts`
- endpoint sensitif sebaiknya mulai tanpa `--skip-auth` agar header autentikasi terisi otomatis

Kita bisa pakai:

- `--operation`: shortcut alias operation (direkomendasikan)
- `--path`: path relatif ke base URL (untuk endpoint yang tidak punya alias)
- `--path-param`: isi placeholder path jika alias/path butuh variable
- `--query`: query param key/value
- `--header`: override header tambahan
- `--base-url`: override base URL untuk testing khusus

Untuk memperbarui alias operasi Xendit dari Postman collection, jalankan:

```bash
./scripts/convert-xendit-postman-to-openapi.sh \
  docs/apis/xendit-openapi-from-postman.json \
  /path/API-Xendit.postman_collection.json \
  /path/API-Xendit\ SNAP.postman_collection.json
./scripts/generate-xendit-openapi-aliases.sh \
  docs/apis/xendit-openapi-from-postman.json \
  internal/cli/xendit_openapi_aliases_generated.go
```

Contoh:

```bash
rutebayar api midtrans --environment sandbox --operation status --path-param order_id=rb-demo-001
rutebayar api midtrans --operation snap-transaction --method POST --body '{"transaction_details":{"order_id":"rb-demo-002","gross_amount":12000}}'
rutebayar api xendit --operation auth-balance
rutebayar api doku --operation order-status --path-param invoice_number_or_request_id=INV-001
rutebayar api doku --method GET --path /orders/v1/status/INV-002
rutebayar api ipaymu --operation payment-channels --method GET
```

Mapping operasi yang didukung (ringkas):

| Provider | Operation | Method | Path |
| - | - | - | - |
| midtrans | auth-test / auth / ping | GET | /v2/rute-bayar-auth-test/status |
| midtrans | status / check-status | GET | /v2/{order_id}/status |
| midtrans | charge / create | POST | /v2/charge |
| midtrans | snap / snap-transaction / snap-v1 | POST | /snap/v1/transactions |
| midtrans | approve / cancel / deny / expire / refund | POST | /v2/{order_id}/<action> |
| xendit | auth-balance / balance | GET | /balance |
| xendit | session-create / create | POST | /sessions |
| xendit | session-status / status | GET | /sessions/{session_id} |
| doku | checkout | POST | /checkout/v1/payment |
| doku | order-status / status | GET | /orders/v1/status/{invoice_number_or_request_id} |
| ipaymu | payment-channels / channels | GET | /api/v2/payment-channels |
| ipaymu | transaction | POST | /api/v2/transaction |

Ringkasan hasil smoke test terakhir:

- `xendit session-create` menolak payload yang belum lengkap (`reference_id` wajib ada).
- `doku` dan `ipaymu` menolak request tanpa header/credential yang tepat.
- `midtrans` status/approve/deny/cancel/expire/refund bisa dipanggil via alias; hasil final bergantung lifecycle transaksi.

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
