---
title: Install and first run
description: Install rutebayar, check the binary, and run your first provider command.
lang: en
order: 1
---

Rute Bayar ships static binaries for Linux, macOS, and Windows. The main fast-start path is the installer script.

```bash
curl -fsSL https://raw.githubusercontent.com/pendig/rute-bayar/main/scripts/install.sh | bash
rutebayar version
rutebayar provider list
```

Need Homebrew, Go install, or a manual binary? Open [install options](/en/docs/install-options/).

For local development from source:

```bash
go test ./...
go build -o ./bin/rutebayar ./cmd/rute-bayar
./bin/rutebayar --help
```

Keep provider credentials outside the repository. Use CLI onboarding to store sandbox credentials in your local SQLite database.
