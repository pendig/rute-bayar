---
title: Why Rute Bayar starts with a CLI
description: The CLI is the smallest stable contract between product code, operations, and payment providers.
lang: en
pubDate: 2026-05-19
---

Payment work needs a surface that is easy to audit. A CLI gives Rute Bayar a narrow, explicit boundary for creating payments, checking status, receiving operational commands, and helping AI Agents work without embedding provider logic.

The daemon handles webhook traffic. The CLI handles human and automation workflows. Together they keep the first version small enough to understand and serious enough to operate.
