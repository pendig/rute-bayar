# Status Mapping

Dokumen ini mencatat mapping status provider ke `domain.PaymentStatus`.
Tujuannya agar perubahan adapter Midtrans/Xendit tetap konsisten dan mudah direview.

## Domain Status

- `pending`: pembayaran/refund masih berjalan atau belum final.
- `paid`: provider menandai pembayaran berhasil, tetapi belum selalu berarti settled.
- `settled`: dana sudah dianggap settled/complete oleh provider.
- `authorized`: pembayaran sudah diotorisasi.
- `captured`: pembayaran sudah dicapture.
- `failed`: provider menandai transaksi gagal/ditolak.
- `expired`: transaksi melewati masa berlaku.
- `cancelled`: transaksi dibatalkan.
- `refunded`: refund penuh berhasil.
- `partial_refunded`: sebagian pembayaran sudah direfund.

## Midtrans

| Provider status | Fraud status | Rute Bayar status | Catatan |
| --- | --- | --- | --- |
| `pending` | any | `pending` | Transaksi menunggu pembayaran/settlement. |
| `settlement` | any | `settled` | Status final sukses untuk banyak metode. |
| `capture` | `accept` | `captured` | Capture sukses. |
| `capture` | selain `accept` | `pending` | Biasanya butuh follow-up fraud/settlement. |
| `deny` | any | `failed` | Ditolak. |
| `failure` | any | `failed` | Gagal. |
| `cancel` | any | `cancelled` | Dibatalkan. |
| `expire` | any | `expired` | Kedaluwarsa. |
| `refund` | any | `refunded` | Refund penuh. |
| `partial_refund` | any | `partial_refunded` | Refund sebagian. |
| unknown | any | `pending` | Fallback konservatif. |

## Xendit Payment Session

| Provider status | Rute Bayar status |
| --- | --- |
| `ACTIVE` | `pending` |
| `PENDING` | `pending` |
| `COMPLETED` | `settled` |
| `SETTLED` | `settled` |
| `SUCCEEDED` | `paid` |
| `SUCCEEDDED` | `paid` |
| `PAID` | `paid` |
| `AUTHORIZED` | `authorized` |
| `CAPTURED` | `captured` |
| `EXPIRED` | `expired` |
| `CANCELLED` | `cancelled` |
| `CANCELED` | `cancelled` |
| `FAILED` | `failed` |
| `REFUNDED` | `refunded` |
| `PARTIAL_REFUNDED` | `partial_refunded` |
| unknown | `pending` |

## Xendit Refund

| Provider status | Rute Bayar status |
| --- | --- |
| `SUCCEEDED` | `refunded` |
| `PENDING` | `pending` |
| `FAILED` | `failed` |
| `CANCELLED` | `cancelled` |
| `CANCELED` | `cancelled` |
| unknown | `pending` |

## DOKU Checkout / Check Status / HTTP Notification

| Provider status | Rute Bayar status |
| --- | --- |
| `ORDER_GENERATE` | `pending` |
| `ORDER_GENERATED` | `pending` |
| `ORDER_RECOVERED` | `pending` |
| `ORDER_EXPIRED` | `expired` |
| `PENDING` | `pending` |
| `SUCCESS` | `paid` |
| `FAILED` | `failed` |
| `EXPIRED` | `expired` |
| `REFUNDED` | `refunded` |
| `PARTIAL_REFUNDED` | `partial_refunded` |
| `TIMEOUT` | `pending` |
| `REDIRECT` | `pending` |
| `CANCELLED` | `cancelled` |
| `CANCELED` | `cancelled` |
| `APPROVE` | `paid` |
| `REJECT` | `failed` |
| unknown | `pending` |

## Implementasi

Normalisasi status provider dilakukan lewat `internal/provider.MapPaymentStatus`.
Adapter tetap menyimpan mapping spesifik provider dekat dengan adapter masing-masing agar konteks dokumentasi provider tidak tersebar terlalu jauh.
