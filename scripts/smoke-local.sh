#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
BIN_PATH="$TMP_DIR/rute-bayar"
DB_PATH="$TMP_DIR/rute-bayar.sqlite3"
WEBHOOK_ADDR="${RUTE_BAYAR_SMOKE_ADDR:-127.0.0.1:18080}"
FORWARD_ADDR="${RUTE_BAYAR_SMOKE_FORWARD_ADDR:-127.0.0.1:18081}"
FORWARDED_FILE="$TMP_DIR/forwarded.json"

daemon_pid=""
receiver_pid=""

cleanup() {
  if [[ -n "$daemon_pid" ]]; then
    kill "$daemon_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$receiver_pid" ]]; then
    kill "$receiver_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

wait_for_url() {
  local url="$1"
  for _ in $(seq 1 40); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  echo "timeout waiting for $url" >&2
  return 1
}

cat >"$TMP_DIR/receiver.go" <<'GO'
package main

import (
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Args[1]
	out := os.Args[2]
	http.HandleFunc("/forward", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(out, body, 0o600); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	log.Fatal(http.ListenAndServe(addr, nil))
}
GO

cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$TMP_DIR/go-build}"

echo "Building local binary..."
go build -o "$BIN_PATH" ./cmd/rute-bayar

export RUTE_BAYAR_DB_PATH="$DB_PATH"
export RUTE_BAYAR_ENV="sandbox"
export RUTE_BAYAR_WEBHOOK_ADDR="$WEBHOOK_ADDR"

echo "Migrating database..."
"$BIN_PATH" db migrate

echo "Onboarding dummy Xendit sandbox credential for local webhook parsing..."
"$BIN_PATH" onboard xendit --secret-key "xnd_test_smoke_only" --environment sandbox >/dev/null

echo "Starting local forwarding receiver on $FORWARD_ADDR..."
go run "$TMP_DIR/receiver.go" "$FORWARD_ADDR" "$FORWARDED_FILE" &
receiver_pid="$!"
sleep 0.5

echo "Configuring forwarding target..."
"$BIN_PATH" webhook forward add \
  --provider xendit \
  --name smoke-forward \
  --url "http://$FORWARD_ADDR/forward" \
  --event-filter "event=payment_session.created" \
  --retry-max-attempts 1 >/dev/null

echo "Starting webhook daemon on $WEBHOOK_ADDR..."
"$BIN_PATH" webhook serve --addr "$WEBHOOK_ADDR" --environment sandbox &
daemon_pid="$!"
wait_for_url "http://$WEBHOOK_ADDR/healthz"

echo "Posting local Xendit webhook simulation..."
curl -fsS -X POST "http://$WEBHOOK_ADDR/webhooks/xendit" \
  -H "Content-Type: application/json" \
  -d '{"event":"payment_session.created","status":"ACTIVE","reference_id":"rb-smoke-001","id":"evt-smoke-001"}' >/dev/null

for _ in $(seq 1 20); do
  if [[ -s "$FORWARDED_FILE" ]]; then
    break
  fi
  sleep 0.25
done

if [[ ! -s "$FORWARDED_FILE" ]]; then
  echo "forwarded webhook payload was not received" >&2
  exit 1
fi

echo "Forwarding attempts:"
"$BIN_PATH" webhook forward attempts list --status success

echo "Smoke test completed."
