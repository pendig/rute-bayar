---
title: Pemasangan dan run pertama
description: Pasang rutebayar, cek binary, dan jalankan command provider pertama.
lang: id
order: 1
---

Rute Bayar tersedia sebagai binary static dan ditargetkan bisa dipasang lewat Homebrew.

```bash
brew tap pendig/tap
brew install rutebayar
rutebayar provider list
```

Untuk development dari source:

```bash
go test ./...
go build -o ./bin/rutebayar ./cmd/rute-bayar
./bin/rutebayar --help
```

Jangan simpan credential provider di repository. Gunakan CLI onboarding untuk menyimpan credential sandbox ke database SQLite lokal.
