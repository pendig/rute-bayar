# Provider Integration

## Tujuan

Dokumen ini menjadi panduan untuk menambahkan provider baru ke Rute Bayar tanpa mengganggu core logic.

## Standar Integrasi

Setiap provider adapter wajib menyediakan:

- create payment
- get payment status
- refund payment
- verify webhook
- parse webhook event
- map status provider ke status internal
- capability declaration

## Mapping Status

Core system harus punya status netral provider, misalnya:

- `pending`
- `paid`
- `failed`
- `expired`
- `cancelled`
- `refunded`
- `partial_refunded`
- `settled`
- `authorized`
- `captured`

Provider-specific status harus selalu dipetakan ke status internal ini.

## Webhook Handling

Untuk setiap provider:

1. Verifikasi webhook sesuai mekanisme resmi provider.
2. Simpan payload mentah dan headers mentah.
3. Cek dedup/idempotency.
4. Normalisasi event.
5. Update payment state.
6. Jika forwarding aktif, kirim payload asli ke target user.

## Reconciliation

Webhook tidak boleh jadi satu-satunya sumber kebenaran.

Jika webhook gagal, terlambat, atau status belum final, sistem harus bisa:

- mengecek status lewat API provider
- membandingkan hasilnya dengan state internal
- memperbarui state bila ada perubahan

## JSON Logging Standard

Untuk debugging, semua provider harus menyimpan:

- outbound request JSON
- outbound response JSON
- inbound webhook JSON
- inbound webhook headers JSON

## Forwarding Policy

- Forwarding bersifat pass-through.
- Payload yang diforward tetap apa adanya dari provider.
- Retry policy default berlaku jika user tidak mengubahnya via CLI.
- Provider adapter tidak perlu tahu target forwarding; logic forwarding berada di daemon/application layer.

## Tambahan Provider Baru

Kalau provider baru ditambahkan nanti, langkah minimum:

1. Tambah adapter baru.
2. Tambah capability registry.
3. Tambah mapping status.
4. Tambah webhook endpoint.
5. Tambah onboarding flow di CLI.
6. Tambah test sandbox.
