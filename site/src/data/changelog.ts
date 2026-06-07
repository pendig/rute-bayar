export const changelogEntries = [
  {
    version: "v0.1.7",
    date: "2026-06-05",
    tag: "iPaymu sandbox release",
    summary:
      "Rilis ini menambahkan provider iPaymu untuk onboarding, payment redirect/direct, status check, webhook parsing, dan reconciliation berbasis SQLite.",
    items: [
      "Credential onboarding iPaymu dengan VA dan API key untuk sandbox/production config.",
      "`pay create`, `pay status`, dan `reconcile` mendukung iPaymu, termasuk sandbox QRIS redirect proof sampai status `paid`.",
      "Webhook `/webhooks/ipaymu` menerima callback provider dan menyimpan raw payload untuk debugging.",
      "Known limitation: signature verification callback form-urlencoded iPaymu masih perlu hardening; gunakan reconciliation sebagai fallback source-of-truth.",
      "Known limitation: refund iPaymu belum tersedia karena API publik iPaymu v2 belum mengekspos endpoint refund resmi/terverifikasi.",
    ],
  },
  {
    version: "v0.1.6",
    date: "2026-06-01",
    tag: "Maintenance release",
    summary:
      "Rilis maintenance ini merapikan dependency CI/GitHub Pages, memperbarui SQLite driver, dan membuat sandbox E2E lebih tahan saat beberapa PR berjalan paralel.",
    items: [
      "GitHub Pages actions diperbarui ke `upload-pages-artifact@v5`, `setup-node@v6`, dan `deploy-pages@v5`.",
      "`modernc.org/sqlite` diperbarui ke v1.51.0.",
      "Reference sandbox E2E dibuat unik per GitHub run attempt agar Xendit tidak kena duplicate reference `409` saat trusted PR checks paralel.",
    ],
  },
  {
    version: "v0.1.5",
    date: "2026-05-30",
    tag: "DOKU release update",
    summary:
      "Rilis update ini menambahkan DOKU Checkout, webhook verification, forwarding pass-through, dan hardening dokumentasi callback agar release lebih siap dipakai.",
    items: [
      "Onboarding DOKU Checkout dengan client ID dan secret key sandbox.",
      "Pay create/status DOKU serta webhook `/webhooks/doku` dengan signature verification.",
      "Webhook forwarding tetap pass-through dan menyimpan raw JSON inbound/outbound untuk debugging.",
      "Docs dan site diselaraskan untuk setup callback dan release flow DOKU.",
    ],
  },
  {
    version: "v0.1.4",
    date: "2026-05-21",
    tag: "Release automation hardening",
    summary:
      "Rilis stabil berikutnya menyempurnakan alur release dan sinkronisasi Homebrew agar lebih aman serta fleksibel untuk proses manual/otomatis.",
    items: [
      "Menambahkan mode manual dispatch untuk release workflow (`workflow_dispatch`) dengan opsi dry-run.",
      "Hardening Homebrew sinkronisasi setelah build release, termasuk perbaikan URL versi.",
      "Mendorong sinkronisasi otomatis formula Homebrew pada proses release agar update rilis tidak manual.",
    ],
  },
  {
    version: "v0.1.3",
    date: "2026-05-20",
    tag: "Landing dan docs polish",
    summary:
      "Penambahan landing site multilingual dan perbaikan UX, termasuk halaman changelog, halaman blog, skill page, dan deployment untuk GitHub Pages.",
    items: [
      "Menyediakan landing page bilingual (ID/EN) dengan fitur dokumentasi dan panduan AI Agent.",
      "Penyelarasan navigasi, sitemap, dan base path untuk deployment GitHub Pages.",
      "Penyesuaian polish untuk branding, spacing, dan struktur halaman publik.",
    ],
  },
  {
    version: "v0.1.2",
    date: "2026-05-19",
    tag: "CI dan provider sandbox",
    summary:
      "Menambahkan workflow E2E sandbox internal untuk provider, memperjelas perbedaan smoke CI dan provider sandbox E2E, serta merapikan naming `rutebayar` pada dokumentasi release.",
    items: [
      "Internal PR-triggered provider sandbox E2E workflow.",
      "Dokumentasi CI menjelaskan smoke local dan sandbox provider E2E.",
      "`go test ./...` tetap hijau di main.",
    ],
  },
  {
    version: "v0.1.1",
    date: "2026-05-19",
    tag: "Rename CLI",
    summary:
      "Menstandarkan command publik ke `rutebayar` di README, docs, quickstart, dan contoh production install setelah rename binary.",
    items: ["Command dan dokumentasi memakai `rutebayar`.", "Contoh Homebrew dan release artifact diselaraskan.", "`go test ./...` hijau setelah merge."],
  },
  {
    version: "v0.1.0",
    date: "2026-05-17",
    tag: "Stable foundation",
    summary:
      "Rilis stabil pertama untuk fondasi CLI dan daemon payment router Indonesia, mencakup Xendit, Midtrans, SQLite, webhook, forwarding, refund, dan release automation.",
    items: [
      "Provider onboarding, pay create/status/refund, reconcile, webhook serve, replay, dan forwarding diagnostics.",
      "Xendit Payment Sessions dan Midtrans Core API untuk flow utama sandbox.",
      "SQLite persistence untuk payment, refund, webhook, dan forwarding.",
      "Release automation untuk Linux, macOS, Windows, dan checksums.",
    ],
  },
  {
    version: "v0.1.0-alpha.3",
    date: "2026-05-08",
    tag: "Sandbox E2E",
    summary:
      "Alpha ketiga berfokus pada pembuktian sandbox E2E, metode refundable Midtrans, validasi tunnel webhook, dan forwarding.",
    items: ["Midtrans card dan dynamic QRIS create flow.", "3DS helper untuk sandbox browser authentication.", "Xendit create, webhook, forwarding, dan refund proof."],
  },
  {
    version: "v0.1.0-alpha.2",
    date: "2026-05-07",
    tag: "Release automation",
    summary:
      "Alpha kedua menambahkan CI, build matrix, release artifacts, checksum, dan command diagnostik forwarding attempt.",
    items: ["GitHub Actions CI untuk format, vet, test, dan build.", "Tag-driven release automation.", "`webhook forward attempts` list/show/retry."],
  },
  {
    version: "v0.1.0-alpha.1",
    date: "2026-05-07",
    tag: "First alpha",
    summary:
      "Alpha pertama membuka fondasi CLI onboarding, payment create/status/refund, reconcile, webhook daemon, signature validation, dan forwarding.",
    items: ["CLI onboarding Midtrans dan Xendit.", "SQLite-backed persistence.", "Webhook daemon dan replay awal."],
  },
];
