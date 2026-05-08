# Security Policy

## Supported Versions

Rute Bayar is currently in alpha. Security fixes target the latest alpha release and the `main` branch.

| Version | Supported |
| --- | --- |
| `v0.1.0-alpha.x` | Best effort |
| Older releases | No |

## Reporting a Vulnerability

Please do not open a public GitHub issue for vulnerabilities.

Report privately by contacting the maintainer:

- Wahyu Adi Putra Pena Digital
- GitHub: https://github.com/pendig/rute-bayar

If GitHub private vulnerability reporting is enabled for the repository, prefer that channel.

Include:

- A clear description of the issue.
- Impact and affected versions or commits.
- Reproduction steps or proof of concept.
- Any known mitigations.

## Secret Handling

Rute Bayar stores provider credentials locally in SQLite during onboarding. Treat local databases, `.env` files, raw webhook payloads, and debug logs as sensitive.

Do not commit:

- `.env` or `.env.*`
- SQLite runtime databases
- Provider API keys
- Webhook verification tokens
- Raw payload dumps that include credentials or customer data

If a credential is exposed, rotate it in the provider dashboard before continuing tests.
