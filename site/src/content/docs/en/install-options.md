---
title: Install options
description: Choose how to install rutebayar through the quick installer, Homebrew, Go install, or release binaries.
lang: en
order: 2
---

The quick installer is the recommended path for most users because the script downloads the matching release binary for your operating system and architecture.

```bash
curl -fsSL https://raw.githubusercontent.com/pendig/rute-bayar/main/scripts/install.sh | bash
rutebayar version
```

## Homebrew

Use Homebrew when you want CLI updates to follow your package manager workflow on macOS or Linux.

```bash
brew tap pendig/tap
brew install rutebayar
rutebayar version
```

## Go install

Use Go install when the Go toolchain is already available and you want to build the binary from the Go module.

```bash
go install github.com/pendig/rute-bayar/cmd/rute-bayar@latest
which rutebayar
rutebayar version
```

Make sure `$GOBIN` or `$GOPATH/bin` is available in your shell `PATH`.

## Manual binary

Use a manual binary for servers, CI, or environments that do not use a package manager. Match the artifact name to your target OS and architecture from the GitHub release page.

```bash
curl -fSL -o rutebayar https://github.com/pendig/rute-bayar/releases/latest/download/rutebayar-linux-amd64
chmod +x rutebayar
./rutebayar --version
```

After the binary is available, inspect providers and continue with sandbox onboarding.

```bash
rutebayar provider list
rutebayar onboard xendit --environment sandbox
```
