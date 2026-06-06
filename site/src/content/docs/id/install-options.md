---
title: Opsi install
description: Pilih cara memasang rutebayar melalui quick installer, Homebrew, Go install, atau binary release.
lang: id
order: 2
---

Quick installer adalah jalur utama untuk kebanyakan pengguna karena script akan mengambil binary release yang sesuai untuk sistem operasi dan arsitektur mesin.

```bash
curl -fsSL https://raw.githubusercontent.com/pendig/rute-bayar/main/scripts/install.sh | bash
rutebayar version
```

## Homebrew

Gunakan Homebrew jika kamu ingin update CLI lewat workflow package manager di macOS atau Linux.

```bash
brew tap pendig/tap
brew install rutebayar
rutebayar version
```

## Go install

Gunakan Go install jika Go toolchain sudah tersedia dan kamu ingin membangun binary dari module Go.

```bash
go install github.com/pendig/rute-bayar/cmd/rute-bayar@latest
which rutebayar
rutebayar version
```

Pastikan `$GOBIN` atau `$GOPATH/bin` sudah masuk ke `PATH` shell kamu.

## Binary manual

Gunakan binary manual untuk server, CI, atau environment yang tidak memakai package manager. Sesuaikan nama artifact dengan OS dan arsitektur target dari halaman release GitHub.

```bash
curl -fSL -o rutebayar https://github.com/pendig/rute-bayar/releases/latest/download/rutebayar-linux-amd64
chmod +x rutebayar
./rutebayar --version
```

Setelah binary tersedia, cek provider dan lanjutkan onboarding sandbox.

```bash
rutebayar provider list
rutebayar onboard xendit --environment sandbox
```
