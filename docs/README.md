# Rute Bayar Docs

Kumpulan dokumentasi awal untuk proyek **Rute Bayar**, payment router berbasis Go yang menjembatani provider pembayaran Indonesia seperti Midtrans dan Xendit.

## Dokumen

- [PRD](./prd.md)
- [Arsitektur](./architecture.md)
- [Model Data](./data-model.md)
- [CLI Onboarding](./cli-onboarding.md)
- [Provider Integration](./provider-integration.md)
- [Implementation Status](./implementation-status.md)
- [Release Readiness](./release-readiness.md)
- [Release Execution Logs](./release/README.md)
- [Implementation Plan](./implementation-plan.md)
- [Webhook Forwarding](./webhook-forwarding.md)
- [Status Mapping](./status-mapping.md)
- [Operations Runbook](./operations-runbook.md)
- [Production Deployment](./production-deployment.md)
- [End-to-End Smoke Test](./end-to-end-smoke.md)
- [Doku Integration Plan](./providers/doku-integration.md)
- [Development](./development.md)
- [Xendit Sandbox Simulation](./xendit-sandbox-simulation.md)
- [Midtrans Sandbox Simulation](./midtrans-sandbox-simulation.md)

## Prinsip Utama

- Modular per provider agar mudah menambah provider baru.
- Semua inbound dan outbound disimpan dalam bentuk JSON mentah untuk debugging dan audit.
- Daemon dapat forward webhook apa adanya sesuai setting user.
- CLI dipakai untuk onboarding, konfigurasi, dan operasi harian.
- Daemon menangani webhook, verifikasi signature, idempotency, dan rekonsiliasi status.
