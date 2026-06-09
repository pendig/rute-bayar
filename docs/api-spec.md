# Rute Bayar API Spec (v1)

Dokumen kontrak API v1 ada di `internal/api/openapi.yaml`.

## Standard Response Envelope

Semua endpoint API v1 mengembalikan JSON dengan envelope:

- `request_id` (UUID) untuk korelasi request.
- `timestamp` (ISO-8601).

Error response memiliki:

- `error_code`
- `message`
- `details` (optional)
- `request_id`
- `timestamp`

## Contoh endpoint utama

### 1) Buat payment

```bash
curl -X POST http://localhost:8080/api/v1/payments \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: <api-key>' \
  -d '{
    "provider":"midtrans",
    "environment":"sandbox",
    "reference":"rb-0001",
    "amount":120000,
    "method":"bank_transfer",
    "channel":"bca"
  }'
```

Respons sukses:

```json
{
  "request_id": "b8f2...",
  "timestamp": "2026-06-07T10:10:10Z",
  "data": {
    "provider":"midtrans",
    "reference":"rb-0001",
    "status":"pending"
  }
}
```

### 2) Reconcile payment

```bash
curl -X POST http://localhost:8080/api/v1/reconcile/midtrans/rb-0001 \
  -H 'X-API-Key: <api-key>'
```

### 3) Daftar webhook event

```bash
curl 'http://localhost:8080/api/v1/webhook-events?provider=midtrans&limit=20' \
  -H 'X-API-Key: <api-key>'
```

## MCP Mapping (ringkas)

- `payment-create` → `POST /api/v1/payments`
- `payment-get` → `GET /api/v1/payments/{reference}`
- `payment-status` → `GET /api/v1/payments/{reference}/status`
- `payment-refund` → `POST /api/v1/payments/{reference}/refund`
- `reconcile-payment` → `POST /api/v1/reconcile/{provider}/{reference}`
- `webhook-event-list` → `GET /api/v1/webhook-events`
- `webhook-event-replay` → `POST /api/v1/webhook-events/{id}/replay`
- `forwarding-target-list` → `GET /api/v1/webhook-forwarding-targets`
- `forwarding-target-crud` → `POST /api/v1/webhook-forwarding-targets`, `PUT /api/v1/webhook-forwarding-targets/{id}`, `DELETE /api/v1/webhook-forwarding-targets/{id}`
