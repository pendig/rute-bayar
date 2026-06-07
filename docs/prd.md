# Product Requirements Document

## Nama Produk

**Rute Bayar**

## Ringkasan

Rute Bayar adalah payment router yang berfungsi sebagai jembatan antara aplikasi merchant dan provider payment gateway Indonesia. Versi awal akan mendukung **Midtrans** dan **Xendit**.

Produk ini tersedia dalam dua mode utama:

- **CLI** untuk onboarding, konfigurasi, pembayaran manual, status check, dan refund jika capability provider tersedia.
- **Daemon** untuk menerima webhook provider, memproses event, dan menyinkronkan status transaksi.

## Tujuan

- Menyediakan satu antarmuka terstandarisasi untuk berbagai provider payment.
- Menyederhanakan integrasi merchant ke banyak provider tanpa menulis logic provider berulang.
- Memudahkan maintenance lewat desain modular dan domain internal yang netral provider.
- Menyimpan seluruh inbound dan outbound request/response dalam format JSON mentah untuk debugging dan audit.

## Non-Tujuan

- Bukan payment processor baru.
- Bukan pengganti dashboard provider.
- Bukan sistem ledger akuntansi penuh pada fase awal.

## Target Pengguna

- Developer yang ingin mengintegrasikan pembayaran melalui satu interface.
- Tim operasional yang butuh status transaksi, retry webhook, dan rekonsiliasi.
- Engineer yang perlu switch provider tanpa mengubah core logic aplikasi.

## Use Case Inti

1. Merchant onboard provider melalui CLI.
2. Merchant membuat payment lewat CLI atau API internal.
3. Provider mengirim webhook ke daemon.
4. Daemon memverifikasi webhook, menyimpan payload mentah, lalu menormalisasi event.
5. Sistem melakukan update status transaksi.
6. Merchant dapat melakukan status inquiry atau refund lewat CLI untuk provider/channel yang mendukung refund.

## Fitur MVP

- Onboarding provider via CLI.
- Konfigurasi credential provider.
- Create payment.
- Cek status payment.
- Refund berbasis capability provider. Xendit/Midtrans punya flow awal; DOKU dan iPaymu tetap unsupported sampai endpoint/capability resmi tersedia.
- Webhook receiver per provider.
- Verifikasi signature/token webhook.
- Idempotency handling.
- Penyimpanan JSON mentah untuk inbound dan outbound.
- Webhook forwarding pass-through yang dapat diatur lewat CLI.
- Rekonsiliasi status via API provider.

## Fitur Lanjutan

- Webhook replay.
- Dead-letter handling untuk event gagal.
- Health check provider.
- Capability registry per provider dan per channel.
- Audit trail dan metrics.
- Support multi-environment: sandbox dan production.

## Acceptance Criteria

- Pengguna dapat onboard provider dari CLI tanpa setup manual yang rumit.
- Setiap request outbound dan response inbound tersimpan sebagai JSON mentah.
- Webhook Midtrans dan Xendit diproses dengan validator masing-masing.
- Status transaksi dapat dipulihkan melalui status API bila webhook gagal.
- Struktur codebase mudah diperluas untuk provider baru.
- Webhook dapat diforward apa adanya sesuai konfigurasi user.

## Rekomendasi Teknologi

- **Bahasa:** Go
- **Database:** SQLite
- **Runtime:** single binary untuk CLI + daemon

## Alasan Pemilihan Go

- Cepat untuk delivery MVP.
- Mudah dirawat dalam jangka panjang.
- Cocok untuk binary tunggal yang menggabungkan HTTP daemon dan CLI.
- Ecosystem HTTP, JSON, dan SQLite sudah matang.
