export const changelogEntries = [
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
