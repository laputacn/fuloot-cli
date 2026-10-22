#!/bin/sh
set -eu
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$ROOT"
. "$ROOT/scripts/fuloot-go.sh"
fuloot_go_setup "$ROOT"
OUTPUT="${OUTPUT:-dist/fuloot-dev}"
mkdir -p "$(dirname -- "$OUTPUT")"
CGO_ENABLED=0 "$GO_CMD" build -mod=readonly -trimpath -o "$OUTPUT" ./cmd/fuloot-dev
fuloot_verify_go "$OUTPUT"
echo "Built $OUTPUT"
