---
title: Pemasangan dan run pertama
description: Pasang rutebayar, cek binary, dan jalankan command provider pertama.
lang: id
order: 1
---

Rute Bayar tersedia sebagai binary static untuk Linux, macOS, dan Windows. Jalur utama untuk mulai cepat adalah script installer.

```bash
curl -fsSL https://raw.githubusercontent.com/pendig/rute-bayar/main/scripts/install.sh | bash
rutebayar version
rutebayar provider list
```

Butuh Homebrew, Go install, atau binary manual? Buka [opsi install](/docs/install-options/).

Untuk development dari source:

```bash
go test ./...
go build -o ./bin/rutebayar ./cmd/rute-bayar
./bin/rutebayar --help
```

Jangan simpan credential provider di repository. Gunakan CLI onboarding untuk menyimpan credential sandbox ke database SQLite lokal.
