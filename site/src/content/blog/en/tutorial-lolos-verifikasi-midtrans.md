---
title: "How to pass Midtrans verification"
description: "A 2026 Midtrans activation checklist covering legal documents, public business proof, and production readiness."
lang: en
pubDate: 2026-06-06
---

Midtrans verification is not just a form submission. The review team needs to see that the business is real, the documents match the entity, and the sales channel can be inspected publicly. This guide is based on official Midtrans documentation reviewed on June 6, 2026.

## Quick summary

Midtrans asks merchants to register through the dashboard, verify email, then complete the activation/passport process. Midtrans also says the submitted website, app, marketplace, or social media URL should be publicly accessible, show the product or service, and display prices.

## Pre-submit checklist

1. Use a business email and phone number that are not already used by another Midtrans, GoBiz, or GoFood account.
2. Make sure the website, app, marketplace, or social media page is active and public.
3. Show clear product or service information.
4. Display prices in IDR.
5. Match the selected business category with the visible business activity.
6. Prepare legal documents according to your business type.

For an individual merchant, Midtrans lists the owner's ID and NPWP as core documents. For entities such as PT, CV, or PMA, prepare the latest company deed, Ministry of Law and Human Rights decree, director ID or passport, director tax ID, company NPWP, NIB/SIUP/TDP, and any additional license required for your business activity.

## How to reduce revision loops

Most activation delays come from incomplete business proof, not API integration. Before submitting, open the business URL in an incognito browser and check whether a reviewer can immediately understand what you sell, how much it costs, and how a customer would buy it.

If you use Instagram or a marketplace, clean up the bio, highlights, catalog, product examples, prices, and contact details. Avoid submitting private, empty, or mismatched pages.

## After approval

After production access is active, separate sandbox and production credentials. For Rute Bayar integrations, store Midtrans credentials in environment configuration or local database, then validate the provider before accepting real transactions.

```bash
rutebayar onboard midtrans --environment production
rutebayar provider list
```

## Official sources

- [How to register as a Midtrans merchant](https://docs.midtrans.com/docs/how-to-register-as-midtrans-merchant)
- [Website or application criteria for Midtrans registration](https://docs.midtrans.com/docs/what-are-the-website-or-application-criterias-for-registering-a-midtrans-account)
- [Legal documents required for Midtrans registration](https://docs.midtrans.com/docs/what-are-the-legal-documents-required-for-midtrans-account-registration)
