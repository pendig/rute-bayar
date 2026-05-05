# Midtrans Sandbox Simulation

## Ringkasan

Simulasi awal dilakukan menggunakan credential sandbox Midtrans untuk menguji dua flow:

- **Snap API** untuk membuat hosted checkout link.
- **Core API** untuk membuat transaksi bank transfer dan mengecek status transaksi.

Credential sandbox tidak disimpan di repository.

## Endpoint Resmi yang Digunakan

```text
POST https://app.sandbox.midtrans.com/snap/v1/transactions
POST https://api.sandbox.midtrans.com/v2/charge
GET https://api.sandbox.midtrans.com/v2/{order_id}/status
```

Referensi dokumentasi resmi:

- https://docs.midtrans.com/reference/snap-api
- https://docs.midtrans.com/reference/charge-transaction
- https://docs.midtrans.com/reference/get-transaction-status

## Snap API

Payload test:

```json
{
  "transaction_details": {
    "order_id": "rb-midtrans-YYYYMMDD-001",
    "gross_amount": 10000
  },
  "customer_details": {
    "first_name": "Test",
    "last_name": "User",
    "email": "test@example.com",
    "phone": "081234567890"
  },
  "credit_card": {
    "secure": true
  },
  "item_details": [
    {
      "id": "item001",
      "price": 10000,
      "quantity": 1,
      "name": "Rute Bayar Midtrans Sandbox Test"
    }
  ]
}
```

Hasil:

- HTTP `201`.
- Response berisi `token`.
- Response berisi `redirect_url` untuk hosted checkout Snap sandbox.

Catatan:

- Status API untuk order Snap yang baru dibuat dapat mengembalikan `status_code: 404` sebelum user memilih/melakukan pembayaran di halaman Snap.
- Untuk Rute Bayar, Snap create response sebaiknya disimpan sebagai payment attempt dengan status internal `pending`.

## Core API Bank Transfer

Payload test:

```json
{
  "payment_type": "bank_transfer",
  "transaction_details": {
    "order_id": "rb-midtrans-core-YYYYMMDD-001",
    "gross_amount": 10000
  },
  "bank_transfer": {
    "bank": "bca"
  },
  "customer_details": {
    "first_name": "Test",
    "last_name": "User",
    "email": "test@example.com",
    "phone": "081234567890"
  }
}
```

Hasil:

- HTTP `200`.
- Response `status_code: 201`.
- Response `transaction_status: pending`.
- Response `fraud_status: accept`.
- Response berisi BCA virtual account number.

Status inquiry untuk order yang sama:

- HTTP `200`.
- Response `status_message: Success, transaction is found`.
- Response `transaction_status: pending`.

## Mapping Awal

- Midtrans `pending` -> Rute Bayar `pending`.
- Midtrans `settlement` -> Rute Bayar `paid` atau `settled`, perlu diputuskan saat implementasi status mapping final.
- Midtrans `capture` + `fraud_status: accept` -> Rute Bayar `captured`.
- Midtrans `deny`, `cancel`, `expire`, dan `failure` perlu dimapping ke status internal yang sesuai.

## Catatan Implementasi

- Basic Auth memakai Server Key sebagai username dan password kosong.
- Semua request dan response harus disimpan sebagai JSON mentah.
- Snap cocok untuk hosted checkout/payment link.
- Core API cocok untuk flow spesifik seperti virtual account, QRIS, dan e-wallet.
- Webhook Midtrans tetap harus diverifikasi dengan signature resmi provider.

## Catatan Keamanan

- Server Key dan Client Key tidak boleh disimpan di repository.
- Gunakan `.env` lokal atau secret manager untuk pengujian berikutnya.
- `.env` sudah masuk `.gitignore`.

