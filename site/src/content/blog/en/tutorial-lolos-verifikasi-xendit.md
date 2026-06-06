---
title: "How to pass Xendit verification"
description: "A 2026 Xendit activation checklist for KYC, Indonesian business documents, authorized representatives, and instant activation."
lang: en
pubDate: 2026-06-06
---

Xendit verification is a business KYC process: who owns or represents the business, whether legal documents are complete, and whether the business activity fits Xendit's policies. This guide is based on official Xendit docs and Indonesia Help Center pages reviewed on June 6, 2026.

## Quick summary

New Xendit accounts can use Test Mode for simulation. To enter Live Mode, merchants must complete activation data, upload required documents, and pass KYC. Xendit's Indonesia Help Center says the full account creation and activation flow can take around one week, while its activation timing article says validation is estimated at 24 hours after complete documents are received.

## Pre-submit checklist

1. Create the account from the Xendit dashboard and verify email.
2. Complete the business profile according to entity type and country.
3. Make sure the authorized representative is legally allowed to represent the business.
4. Prepare a clear, valid identity document for the representative.
5. Prepare proof of address if the identity document does not contain the residential address.
6. Prepare proof of authorization if the representative is not already recognized in the legal documents.
7. Make sure the browser and camera are ready for liveness verification.

For Indonesia, Xendit maintains a dedicated business documents page updated on January 5, 2026. Because requirements can vary by entity type and product, use that page as the final reference before submission.

## Instant activation is not full approval

Xendit has Instant Activation for some low-risk Indonesian individual businesses that do not require additional licensing. Eligible accounts may accept Money-In transactions sooner. It does not immediately unlock Money-Out, withdrawals, or every additional channel; Xendit still completes document KYC first.

## How to avoid rejection

Keep all data consistent: legal name, brand name, tax or business registration, settlement bank account, address, and business website should not contradict each other. If Xendit emails an information update request, update every requested item through the dashboard, not just one document.

For Rute Bayar integrations, stay in Test Mode until Live access is fully active and production keys are available.

```bash
rutebayar onboard xendit --environment sandbox
rutebayar onboard xendit --environment production
```

## Official sources

- [Verifying your account - Xendit Docs](https://docs.xendit.co/docs/verifying-your-account)
- [Authorized representative requirements - Xendit Docs](https://docs.xendit.co/docs/authorized-representative-requirements)
- [How long does Xendit account activation take?](https://help.xendit.co/hc/id/articles/4412730514841-Berapa-Lama-Proses-Aktivasi-Akun-Xendit)
- [What is Instant Activation?](https://help.xendit.co/hc/en-us/articles/4415380128025-What-is-Instant-Activation)
