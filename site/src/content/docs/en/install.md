---
title: Install and first run
description: Install rutebayar, check the binary, and run your first provider command.
lang: en
order: 1
---

Rute Bayar ships as static binaries and is also intended to be installable through Homebrew.

```bash
brew tap pendig/tap
brew install rutebayar
rutebayar provider list
```

For local development from source:

```bash
go test ./...
go build -o ./bin/rutebayar ./cmd/rute-bayar
./bin/rutebayar --help
```

Keep provider credentials outside the repository. Use CLI onboarding to store sandbox credentials in your local SQLite database.
