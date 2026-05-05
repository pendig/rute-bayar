# Rute Bayar Docs

Kumpulan dokumentasi awal untuk proyek **Rute Bayar**, payment router berbasis Go yang menjembatani provider pembayaran Indonesia seperti Midtrans dan Xendit.

## Dokumen

- [PRD](./prd.md)
- [Arsitektur](./architecture.md)
- [Model Data](./data-model.md)
- [CLI Onboarding](./cli-onboarding.md)
- [Provider Integration](./provider-integration.md)
- [Webhook Forwarding](./webhook-forwarding.md)
- [Development](./development.md)
- [Xendit Sandbox Simulation](./xendit-sandbox-simulation.md)

## Prinsip Utama

- Modular per provider agar mudah menambah provider baru.
- Semua inbound dan outbound disimpan dalam bentuk JSON mentah untuk debugging dan audit.
- Daemon dapat forward webhook apa adanya sesuai setting user.
- CLI dipakai untuk onboarding, konfigurasi, dan operasi harian.
- Daemon menangani webhook, verifikasi signature, idempotency, dan rekonsiliasi status.
