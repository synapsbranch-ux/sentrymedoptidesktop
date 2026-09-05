#!/bin/sh
set -eu

e2e_data_dir="${SENTRYMED_E2E_DATA_DIR:-$(mktemp -d)}"
export SENTRYMED_DATA_DIR="$e2e_data_dir"
export SENTRYMED_ADDRESS="${SENTRYMED_ADDRESS:-127.0.0.1:8787}"

cleanup() {
  if [ -z "${SENTRYMED_E2E_DATA_DIR:-}" ]; then
    rm -rf "$e2e_data_dir"
  fi
}
trap cleanup EXIT INT TERM

go run ./cmd/sentrymed --seed
exec go run ./cmd/sentrymed
