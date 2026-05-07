#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BIN_PATH="${RUTE_BAYAR_E2E_BIN:-$TMP_DIR/rute-bayar}"
DB_PATH="${RUTE_BAYAR_E2E_DB_PATH:-$TMP_DIR/rute-bayar-e2e.sqlite3}"
AMOUNT="${RUTE_BAYAR_E2E_AMOUNT:-15000}"
RUN_XENDIT="${RUTE_BAYAR_E2E_XENDIT:-1}"
RUN_MIDTRANS="${RUTE_BAYAR_E2E_MIDTRANS:-1}"

cleanup() {
  if [[ -z "${RUTE_BAYAR_E2E_KEEP_DB:-}" ]]; then
    rm -rf "$TMP_DIR"
  else
    printf 'kept temporary directory: %s\n' "$TMP_DIR"
  fi
}
trap cleanup EXIT

require_env() {
  local missing=0
  for key in "$@"; do
    if [[ -z "${!key:-}" ]]; then
      printf 'missing required env: %s\n' "$key" >&2
      missing=1
    fi
  done
  return "$missing"
}

run_step() {
  printf '\n==> %s\n' "$1"
  shift
  "$@"
}

masked_presence_report() {
  for key in "$@"; do
    if [[ -n "${!key:-}" ]]; then
      printf '%s=set\n' "$key"
    else
      printf '%s=missing\n' "$key"
    fi
  done
}

cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$TMP_DIR/go-build}"
export RUTE_BAYAR_ENV=sandbox
export RUTE_BAYAR_DB_PATH="$DB_PATH"

printf 'Rute Bayar sandbox E2E\n'
printf 'database: %s\n' "$DB_PATH"
masked_presence_report \
  XENDIT_SECRET_KEY \
  XENDIT_WEBHOOK_TOKEN \
  MIDTRANS_MERCHANT_ID \
  MIDTRANS_CLIENT_KEY \
  MIDTRANS_SERVER_KEY

if [[ "$RUN_XENDIT" == "1" ]]; then
  require_env XENDIT_SECRET_KEY || missing_env=1
fi
if [[ "$RUN_MIDTRANS" == "1" ]]; then
  require_env MIDTRANS_MERCHANT_ID MIDTRANS_CLIENT_KEY MIDTRANS_SERVER_KEY || missing_env=1
fi
if [[ "${missing_env:-0}" == "1" ]]; then
  printf '\nSet rotated sandbox credentials as environment variables, then rerun this script.\n' >&2
  exit 1
fi

if [[ ! -x "$BIN_PATH" ]]; then
  run_step "build binary" go build -o "$BIN_PATH" ./cmd/rute-bayar
fi

run_step "migrate database" "$BIN_PATH" db migrate

if [[ "$RUN_XENDIT" == "1" ]]; then
  xendit_ref="${RUTE_BAYAR_E2E_XENDIT_REF:-rb-e2e-xendit-$(date +%Y%m%d%H%M%S)}"
  xendit_onboard=("$BIN_PATH" onboard xendit --secret-key "$XENDIT_SECRET_KEY" --environment sandbox)
  if [[ -n "${XENDIT_WEBHOOK_TOKEN:-}" ]]; then
    xendit_onboard+=(--webhook-token "$XENDIT_WEBHOOK_TOKEN")
  fi

  run_step "onboard xendit" "${xendit_onboard[@]}"
  run_step "provider test xendit" "$BIN_PATH" provider test xendit
  run_step "pay create xendit" "$BIN_PATH" pay create \
    --provider xendit \
    --method payment_link \
    --reference "$xendit_ref" \
    --amount "$AMOUNT"
  run_step "pay status xendit" "$BIN_PATH" pay status \
    --provider xendit \
    --reference "$xendit_ref"

  if [[ -n "${RUTE_BAYAR_E2E_XENDIT_REFUND_REFERENCE:-}" ]]; then
    run_step "pay refund xendit existing settled reference" "$BIN_PATH" pay refund \
      --provider xendit \
      --reference "${RUTE_BAYAR_E2E_XENDIT_REFUND_REFERENCE}" \
      --provider-reference "${RUTE_BAYAR_E2E_XENDIT_REFUND_PROVIDER_REFERENCE:-}" \
      --amount "${RUTE_BAYAR_E2E_XENDIT_REFUND_AMOUNT:-$AMOUNT}" \
      --refund-reference "rb-e2e-xendit-refund-$(date +%s)"
  else
    printf '\nSKIP xendit refund: set RUTE_BAYAR_E2E_XENDIT_REFUND_REFERENCE for a paid/settled sandbox payment.\n'
  fi
fi

if [[ "$RUN_MIDTRANS" == "1" ]]; then
  midtrans_ref="${RUTE_BAYAR_E2E_MIDTRANS_REF:-rb-e2e-midtrans-$(date +%Y%m%d%H%M%S)}"

  run_step "onboard midtrans" "$BIN_PATH" onboard midtrans \
    --merchant-id "$MIDTRANS_MERCHANT_ID" \
    --client-key "$MIDTRANS_CLIENT_KEY" \
    --server-key "$MIDTRANS_SERVER_KEY" \
    --environment sandbox
  run_step "provider test midtrans" "$BIN_PATH" provider test midtrans
  run_step "pay create midtrans" "$BIN_PATH" pay create \
    --provider midtrans \
    --method bank_transfer \
    --bank "${RUTE_BAYAR_E2E_MIDTRANS_BANK:-bca}" \
    --reference "$midtrans_ref" \
    --amount "$AMOUNT"
  run_step "pay status midtrans" "$BIN_PATH" pay status \
    --provider midtrans \
    --reference "$midtrans_ref"

  if [[ -n "${RUTE_BAYAR_E2E_MIDTRANS_REFUND_REFERENCE:-}" ]]; then
    run_step "pay refund midtrans existing refundable reference" "$BIN_PATH" pay refund \
      --provider midtrans \
      --reference "${RUTE_BAYAR_E2E_MIDTRANS_REFUND_REFERENCE}" \
      --provider-reference "${RUTE_BAYAR_E2E_MIDTRANS_REFUND_PROVIDER_REFERENCE:-}" \
      --amount "${RUTE_BAYAR_E2E_MIDTRANS_REFUND_AMOUNT:-$AMOUNT}" \
      --refund-reference "rb-e2e-midtrans-refund-$(date +%s)"
  else
    printf '\nSKIP midtrans refund: set RUTE_BAYAR_E2E_MIDTRANS_REFUND_REFERENCE for a settled/refundable sandbox payment.\n'
  fi
fi

printf '\nSandbox E2E completed.\n'
