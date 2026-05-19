# Doku Integration (Next Provider)

Dokumen ini menjadi rencana dan checklist awal untuk integrasi **Doku** sebagai provider berikutnya setelah Midtrans dan Xendit.

## Goal

- Menyediakan `pay create`, `pay status`, dan `pay refund` (jika didukung) untuk `--provider doku`.
- Menyinkronkan status dari callback dan polling status API Doku ke status internal.
- Menangani webhook Doku dengan verifikasi signature (jika didukung dokumentasi).
- Menambahkan dukungan forwarding webhook pass-through.
- Menyimpan semua inbound/outbound JSON payload untuk debugging.

## Onboarding

1. Kumpulkan dokumentasi resmi Doku yang akan dipakai:
   - API base URL dan environment sandbox/production.
   - Mekanisme autentikasi (API key/client key atau signature).
   - Event webhook + header verifikasi.
   - API untuk status payment dan refund.
2. Buat template kredensial provider di CLI:
   - `--api-key`
   - `--api-secret` (jika diperlukan)
   - `--environment`
   - `--merchant-id` (jika diperlukan oleh Doku)
3. Buat account provisioning sesuai pola umum provider di proyek ini:
   - `provider_code = doku`
   - `provider_name = Doku`
   - `environment = sandbox|production`
4. Validasi awal:
   - command `provider test doku` memanggil endpoint ringan (health/status) untuk memastikan kredensial benar.

## Command Surface

- `rutebayar provider add doku`
- `rutebayar provider test doku`
- `rutebayar pay create --provider doku ...`
- `rutebayar pay status --provider doku --reference <ref>`
- `rutebayar pay refund --provider doku --reference <ref> [--amount <amount>]`
- `rutebayar webhook forward add --provider doku --name ... --url ...`
- `rutebayar webhook replay --provider doku --event-id ...`

## Runtime Mapping

- Buat adapter `internal/provider/doku` sesuai pola adapter saat ini.
- Tambah mapping status internal (contoh: pending, paid, failed, expired, refunded, partial_refunded, settled).
- Simpan:
  - outbound request JSON (`request_json`)
  - outbound response JSON (`response_json`)
  - inbound webhook JSON (`inbound_json`)
  - inbound headers JSON (`inbound_headers`)
- Forwarding tetap pass-through tanpa transformasi payload.

## Webhook

- Endpoint: `/webhooks/doku`
- Verifikasi signature berdasarkan dokumentasi resmi Doku.
- Deduplicate event berbasis event ID/transaction ID jika tersedia.
- Simpan event agar replay bisa dijalankan ulang dari DB.
- Update status pembayaran internal setelah validasi event.

## Non-Goals (MVP)

- Tidak menambahkan provider-specific logic ke core service.
- Tidak membuat UI atau API baru dalam step ini (tetap CLI + daemon).
- Tidak mengubah struktur storage yang sudah berjalan.

## Open Questions

- Apakah Doku menyediakan endpoint status yang stabil untuk semua jenis payment method?
- Apakah refund bisa dijalankan per full/partial secara langsung dari payment reference?
- Apakah sandbox Doku menyediakan simulasi status callback untuk seluruh metode pembayaran?

## DoD

- Adapter Doku berhasil dipakai dari CLI untuk minimal 1 method payment.
- Webhook Doku bisa diterima, diverifikasi, dan memicu update internal status.
- `pay status` Doku tersedia untuk mengecek status langsung.
- E2E checklist terpisah bisa dijalankan (create → webhook → pay status → reconcile/refund jika memungkinkan).
