---
title: "How to pass DOKU verification"
description: "A 2026 DOKU activation checklist for Business Account data, legal documents, business proof, and unverified account limits."
lang: en
pubDate: 2026-06-06
---

DOKU Business Account verification determines whether settlement and all dashboard features can be used fully. This guide is based on official DOKU documentation reviewed on June 6, 2026.

## Quick summary

DOKU asks merchants to activate a Business Account, fill in business data, upload legal documents, and wait for verification. DOKU docs say verification can take up to 48 hours after all required documents are uploaded. Personal and Corporate merchants may start accepting certain payments after Business Account activation, but settlement is processed only after the account is successfully verified.

## Pre-submit checklist

1. Choose the right Business Account type: Personal, Corporate, or International.
2. Keep business, representative, and brand data consistent.
3. Upload business proof that clearly shows location, activity, or product.
4. Prepare legal documents according to the account type.
5. Make sure documents are readable, not blurry, uncensored, not expired, and owned by the company.
6. Use PDF, JPG, JPEG, or PNG files with a maximum size of 15 MB.
7. Check status and revision notes from the DOKU Dashboard.

For Corporate accounts, DOKU lists documents such as NIB, company deed and amendments, Ministry of Law and Human Rights decree, business proof photo, NPWP, and director ID. Some business lines require additional documents, such as an OJK license for peer-to-peer lending or a Bank Indonesia license for PSP/PJSP businesses.

## Understand unverified account limits

DOKU limits unverified accounts. Its documentation says Personal Merchants can receive up to 5 transactions with a maximum total volume of IDR 1,000,000, while Corporate Merchants can receive up to 5 transactions with a maximum total volume of IDR 10,000,000. International Merchants must complete verification before accepting payments.

## How to avoid a long Under Review state

If the account remains Under Review, check the Settings section in DOKU Dashboard. In Business Info and Documents, DOKU may show a yellow banner for data still being verified or a red banner for rejected data that must be revised. Avoid partial fixes; resolve every note so the review does not restart from the beginning.

For Rute Bayar integrations, activate sandbox first, then move production credentials only after the Business Account and required payment channels are ready.

```bash
rutebayar onboard doku --environment sandbox
rutebayar onboard doku --environment production
```

## Official sources

- [Activate Business - DOKU Docs](https://docs.doku.com/get-started/activate-business)
- [Business Account - DOKU Docs](https://docs.doku.com/get-started/activate-business/business-account)
- [Requirements and Limitations - DOKU Docs](https://docs.doku.com/accept-payments/payment-methods/requirements-and-limitations)
