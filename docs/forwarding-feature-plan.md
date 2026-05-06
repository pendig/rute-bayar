# Forwarding Feature Plan

Dokumen ini menjabarkan rencana implementasi fitur **webhook forwarding** agar pass-through, stabil, dan bisa dikelola via CLI.

## Tujuan Fitur

- Meneruskan webhook provider secara utuh (tanpa transformasi payload).
- Forwarding bisa dinyalakan/dimatikan per target.
- Target dan retry policy dikelola sepenuhnya lewat CLI.
- Semua request/response forwarding tetap tercatat dalam JSON untuk observability.
- Gagal forwarding tidak mengganggu penerimaan webhook utama (`/webhooks/{provider}`).

## Keadaan Saat Ini

- Skema DB sudah punya:
  - `webhook_forwarding_targets`
  - `webhook_forwarding_attempts`
- Service forwarding sudah ada (`internal/forwarding`) dengan:
  - daftar target enabled
  - retry loop + backoff + timeout
  - penyimpanan attempt
- CLI forwarding saat ini masih scaffold:
  - `rute-bayar webhook forward list|add|update|remove`
- Daemon sudah memanggil forwarding setelah event di-reconcile/record.

## Arsitektur Target

### 1) Store Layer

Tambahkan metode baru di `internal/storage/sqlite/store.go` dan interface:

- `UpsertForwardingTarget(ctx, forwarding.Target) (string, error)`
- `GetForwardingTarget(ctx, targetID string) (forwarding.Target, error)`
- `ListForwardingTargets(ctx, provider domain.ProviderCode) ([]forwarding.Target, error)`
- `DeleteForwardingTarget(ctx, targetID string) error`
- `UpdateForwardingTarget(ctx, forwarding.Target) error` (atau Upsert+ID)
- `ListForwardingAttempt(ctx, webhookEventID string)` (opsional untuk replay/diagnosis)

Catatan:
- `event_filter_json` dan `auth_json` sudah ada di DB; gunakan field ini.
- `event_filter` bisa mulai dari format map sederhana, contoh:
  - `{"event_type": "capture"}`  
  - `{"status": "paid"}`  
  - bisa juga kosong untuk `*` (forward semua).

### 2) Forwarding Service

- `Target.EventFilter` di-resolve dari `event_filter_json`.
- Terapkan filter sebelum forward:
  - jika event tidak match filter, skip target (catat status `skipped`).
- Default retry tetap:
  - `max_attempts: 3`
  - `timeout: 10s`
  - `backoff: 2s`
- CLI dapat override retry policy per target.
- Pada failure, forwarder tetap mengembalikan error agar daemon bisa reply warning (saat ini sudah jalan).

### 3) CLI

Implementasi command `webhook forward`:

- `rute-bayar webhook forward add`
  - Flags:
    - `--provider midtrans|xendit`
    - `--name`
    - `--url`
    - `--enabled` (default `true`)
    - `--event` (repeatable, contoh `capture`, `settlement`, `invoice.updated`)
    - `--status` (repeatable)
    - `--header "Authorization: Bearer ..."` (repeatable)
    - `--retry-max-attempts`
    - `--retry-timeout`
    - `--retry-backoff`
  - Menyimpan `Target` ke SQLite.
- `rute-bayar webhook forward list`
  - list per provider + status enabled
  - tampilkan retry policy ringkas
- `rute-bayar webhook forward update <target-id>`
  - updatable field: nama/url/aktif, filter, header, retry policy
- `rute-bayar webhook forward remove <target-id>`
- `rute-bayar webhook forward replay --event-id <webhook_event_id>`
  - mengeksekusi kembali forward untuk event lama (pakai JSON payload dari DB).

### 4) Webhook Command UX

- `webhook replay` bisa dipakai untuk:
  - kirim ulang webhook event tertentu
  - lihat attempt sebelumnya (opsional `--raw`)
- `webhook serve` tetap:
  - verify -> parse -> reconcile -> record -> forward (best effort).

## Data Model Update (Direkomendasikan)

- `forwarding.Target.RetryPolicy` tetap berisi `max_attempts`, `timeout`, `backoff`.
- `forwarding.Attempt` bisa ditambah kolom:
  - `Status: skipped|success|failed`
  - `ErrorText: string` (jika ingin lebih rich; optional)
- `attempt` bisa di-query berdasarkan target untuk observability.

## Tes yang Harus Dibuat

1. `internal/forwarding`:
   - Filter event tidak cocok → skip tanpa call HTTP.
   - Retry policy: fail dua kali lalu sukses di attempt ke-3.
   - Timeout policy menghentikan request dan dicatat attempt failed.
2. `internal/storage/sqlite`:
   - CRUD target forwarding.
   - Event filter tersimpan dan dibaca utuh.
   - Rekam attempt per target + webhook event.
3. `internal/cli`:
  - add/list/update/remove command.
4. `internal/daemon`:
   - forwarding aktif dan tidak aktif.
   - warning path tetap `accepted` saat forwarding warning.
   - replay command memanggil forwarding target sesuai event/target yang diminta.

## Saran Urutan Pengerjaan

1. Implementasi store CRUD forwarding target + parsing filter/headers.
2. Implementasi CLI add/list/update/remove.
3. Hubungkan filtering di `forwarding.Service`.
4. Tambahkan replay/diagnostic command.
5. Tambahkan unit/integration test dan dokumen ops.

## Acceptance Criteria

- `rute-bayar webhook forward add/list/update/remove` jalan end-to-end.
- Forwarding default pass-through dan bisa di-nonaktifkan per target.
- Retry policy dapat diubah di CLI dan tervalidasi saat run.
- Setiap forwarding attempt terekam `webhook_forwarding_attempts`.
- Jika target gagal, command tetap aman (daemon tidak crash dan tetap mengembalikan response sesuai kebutuhan sistem).
- `go test ./...` tetap green.
