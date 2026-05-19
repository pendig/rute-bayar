# Issue #36 Real Webhook + Refund E2E Execution Log

Tanggal: 2026-05-12

## Tujuan
Menutup acceptance:
- `create -> webhook -> pay status` untuk provider sandbox real (Xendit & Midtrans) melalui endpoint publik sementara.
- Refund E2E untuk transaksi yang sudah settled/refundable.

## Hasil Eksekusi Lanjutan

### 1) Smoke lokal dan verifikasi CLI
- `go test ./...` lulus di branch `main`.
- `scripts/smoke-local.sh` berhasil setelah penyesuaian port sementara dan cache module lokal.
  - port 18080/18081 bentrok, diganti ke `18180/18181`.
  - forwarding attempt untuk simulasi Xendit masuk dan tercatat `success`.
  - output menunjukkan `webhook_forwarding_attempts` dan payload diterima.

### 2) Real status sandbox (on top of DB real-run)
- `go run ./cmd/rute-bayar pay status` dieksekusi untuk reference yang dibuat pada run real:
  - `rb-realblock-xendit-20260512212847` (provider ref `ps-6a0339200168694c2c2a0231`) -> `pending`.
  - `rb-realblock-midtrans-20260512212855` -> `pending`.
- Artinya transaksi belum masuk fase final/settlement saat pengecekan, jadi path refund belum valid.

### 3) Refund call pada status pending
- `pay refund` Xendit (tanpa settlement) mengembalikan:
  - `xendit refund returned status 400`.
- `pay refund` Midtrans (pending) mengembalikan:
  - `midtrans refund returned status_code 412: Merchant cannot modify the status of the transaction.`
- Kode perilaku ini valid sebagai guard agar refund hanya dijalankan untuk transaksi final/refundable.

### 4) Webhook real belum terverifikasi penuh
- Pada titik ini belum didapat bukti callback nyata dari provider ke endpoint publik Cloudflare.
- `webhook_events` pada DB run real belum berubah setelah status check otomatis.

## Langkah Lanjutan untuk Menutup Issue
1. Selesaikan pembayaran sandbox sampai status **paid/settlement** untuk masing-masing provider (sesuai metode yang dipilih).
2. Jalankan daemon pada address yang akan ditunnel:
   - `rutebayar webhook serve --addr 127.0.0.1:8080 --environment sandbox`.
3. Pastikan webhook URL aktif via address daemon yang sama:
   - `wrangler tunnel quick-start http://127.0.0.1:8080`.
4. Arahkan URL provider webhook ke:
   - `https://<domain>.trycloudflare.com/webhooks/xendit`
   - `https://<domain>.trycloudflare.com/webhooks/midtrans`
5. Trigger callback dengan menyelesaikan flow pembayaran.
6. Verifikasi:
   - event masuk ke `webhook_events`,
   - status lokal berubah sesuai final status,
   - dan `pay status` / `reconcile` sinkron.
7. Simpan reference ter-eligible:
   - `RUTE_BAYAR_E2E_XENDIT_REFUND_REFERENCE`
   - `RUTE_BAYAR_E2E_MIDTRANS_REFUND_REFERENCE`
8. Jalankan `scripts/e2e-sandbox.sh` ulang dengan env refund reference agar refund E2E tuntas.
