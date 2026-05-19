# Xendit Sandbox Simulation

## Ringkasan

Simulasi awal dilakukan menggunakan Xendit development secret key untuk membuat **Payment Session** mode `PAYMENT_LINK`.

Endpoint resmi yang digunakan:

```text
POST https://api.xendit.co/sessions
GET https://api.xendit.co/sessions/{session_id}
GET https://api.xendit.co/balance
```

Referensi dokumentasi resmi:

- https://docs.xendit.co/apidocs/create-session
- https://docs.xendit.co/apidocs/get-session
- https://docs.xendit.co/apidocs/get-balance
- https://docs.xendit.co/docs/payment-sessions-overview
- https://docs.xendit.co/apidocs/webhook-notification-sent-defined-webhook-url-updates-payment-session

## Payload Create Session

Payload minimal yang berhasil untuk Indonesia:

```json
{
  "reference_id": "rbtest_YYYYMMDD_001",
  "session_type": "PAY",
  "mode": "PAYMENT_LINK",
  "amount": 10000,
  "currency": "IDR",
  "country": "ID",
  "customer": {
    "reference_id": "rbcustYYYYMMDD001",
    "type": "INDIVIDUAL",
    "email": "test@example.com",
    "mobile_number": "+6281234567890",
    "individual_detail": {
      "given_names": "Test",
      "surname": "User"
    }
  },
  "items": [
    {
      "reference_id": "item001",
      "name": "Rute Bayar Test",
      "type": "DIGITAL_PRODUCT",
      "category": "SOFTWARE",
      "net_unit_amount": 10000,
      "quantity": 1,
      "currency": "IDR"
    }
  ],
  "capture_method": "AUTOMATIC",
  "locale": "id",
  "description": "Rute Bayar Xendit sandbox simulation",
  "success_return_url": "https://example.com/success",
  "cancel_return_url": "https://example.com/cancel"
}
```

## Temuan

- Basic Auth harus memakai secret key sebagai username dan password kosong.
- `GET /balance` dipakai sebagai probe auth ringan untuk `provider test xendit`.
- Jika Xendit membalas `403` pada `/balance`, credential tetap bisa valid untuk payment flow tetapi tidak punya permission balance read. CLI harus menampilkan warning, bukan langsung menganggap API key invalid.
- `items[].category` wajib dikirim. Tanpa field ini Xendit membalas `API_VALIDATION_ERROR`.
- Response create session sukses mengembalikan HTTP `201`.
- Response status session mengembalikan HTTP `200`.
- Status awal Payment Session adalah `ACTIVE`.
- Untuk status internal Rute Bayar, `ACTIVE` sebaiknya dipetakan ke `pending`.
- Response berisi `payment_link_url` yang mengarah ke hosted checkout Xendit development.
- Session default pada simulasi ini expired sekitar 30 menit setelah dibuat.

## Webhook Proof

Untuk Payment Sessions, webhook URL perlu dikonfigurasi melalui Xendit Dashboard sandbox/development. Arahkan URL Payment Sessions ke:

```text
https://<public-domain>/webhooks/xendit
```

Daemon memvalidasi header `X-Callback-Token` jika token disimpan saat onboarding:

```bash
rutebayar onboard xendit \
  --secret-key "$XENDIT_SECRET_KEY" \
  --webhook-token "$XENDIT_WEBHOOK_TOKEN" \
  --environment sandbox
```

Rencana bukti lengkap dicatat di [Issue #39 Xendit Webhook Proof Plan](./release/issue-39-xendit-webhook-proof-plan.md).

## Catatan Keamanan

- Secret API key tidak boleh disimpan di repository.
- Gunakan `.env` lokal atau secret manager untuk pengujian berikutnya.
- `.env` sudah masuk `.gitignore`.
