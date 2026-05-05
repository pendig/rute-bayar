# Data Model

## Tujuan

Skema data harus sederhana, audit-friendly, dan mudah dipindah ke database lain nanti jika diperlukan.

## Entitas Utama

### providers

Menyimpan daftar provider yang didukung.

Field yang disarankan:

- `id`
- `name`
- `code`
- `status`
- `created_at`
- `updated_at`

### provider_accounts

Menyimpan credential dan konfigurasi per merchant/provider.

Field yang disarankan:

- `id`
- `provider_id`
- `environment`
- `display_name`
- `credential_json`
- `config_json`
- `created_at`
- `updated_at`

Catatan:

- `credential_json` menyimpan key, secret, token, atau field sensitif lain dalam format JSON terstruktur.
- `config_json` menyimpan setting seperti webhook secret, callback URL, dan channel config.

### payment_intents

Menyimpan objek payment internal yang netral provider.

Field yang disarankan:

- `id`
- `external_ref`
- `provider_id`
- `amount`
- `currency`
- `status`
- `metadata_json`
- `created_at`
- `updated_at`

### payment_attempts

Menyimpan setiap percobaan create payment ke provider.

Field yang disarankan:

- `id`
- `payment_intent_id`
- `provider_id`
- `request_json`
- `response_json`
- `status`
- `provider_reference`
- `created_at`
- `updated_at`

Catatan penting:

- `request_json` adalah outbound mentah ke provider.
- `response_json` adalah inbound mentah dari provider.
- Ini wajib untuk debugging, audit, dan replay.

### webhook_events

Menyimpan event webhook masuk.

Field yang disarankan:

- `id`
- `provider_id`
- `provider_event_id`
- `event_type`
- `signature_valid`
- `payload_json`
- `headers_json`
- `received_at`
- `processed_at`
- `processing_status`

Catatan penting:

- `payload_json` menyimpan body mentah webhook.
- `headers_json` menyimpan header mentah webhook.
- `provider_event_id` dipakai untuk idempotency / dedup jika tersedia.

### webhook_forwarding_targets

Menyimpan target forwarding yang dikonfigurasi user.

Field yang disarankan:

- `id`
- `name`
- `provider_id`
- `event_filter_json`
- `target_url`
- `auth_json`
- `retry_policy_json`
- `enabled`
- `created_at`
- `updated_at`

Catatan:

- `event_filter_json` bisa berisi filter provider atau event tertentu.
- `auth_json` menyimpan header atau token autentikasi tujuan forwarding.
- `retry_policy_json` menyimpan retry default yang bisa diubah lewat CLI.

### webhook_forwarding_attempts

Menyimpan setiap percobaan pengiriman ulang webhook ke target forwarding.

Field yang disarankan:

- `id`
- `webhook_event_id`
- `forwarding_target_id`
- `request_json`
- `response_json`
- `status`
- `attempt_no`
- `created_at`
- `updated_at`

### refunds

Menyimpan request refund dan hasilnya.

Field yang disarankan:

- `id`
- `payment_intent_id`
- `provider_id`
- `amount`
- `status`
- `request_json`
- `response_json`
- `provider_reference`
- `created_at`
- `updated_at`

### audit_logs

Menyimpan jejak aktivitas penting.

Field yang disarankan:

- `id`
- `actor_type`
- `actor_id`
- `action`
- `target_type`
- `target_id`
- `detail_json`
- `created_at`

## JSON Storage Policy

Semua payload penting disimpan apa adanya:

- outbound request
- inbound response
- inbound webhook
- headers webhook
- metadata tambahan

Tujuan:

- debug lebih cepat
- replay event lebih mudah
- audit lebih jelas
- migrasi provider lebih aman

## Index yang Disarankan

- `payment_intents.external_ref`
- `payment_attempts.payment_intent_id`
- `payment_attempts.provider_reference`
- `webhook_events.provider_event_id`
- `webhook_events.event_type`
- `webhook_forwarding_targets.provider_id`
- `webhook_forwarding_attempts.webhook_event_id`
- `refunds.payment_intent_id`
