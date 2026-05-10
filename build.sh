#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST_DIR="/vm/project/Project.dev/SDC5/Setup/AdditionalFile/Bin/64"

TARGETS=(
  "linux-amd64"
  "linux-arm64"
)

cd "$ROOT_DIR"

mkdir -p "$DEST_DIR"

for target in "${TARGETS[@]}"; do
  make "$target"
done

install -m 0755 \
  "$ROOT_DIR/bin/sdc-mihomo-linux-amd64" \
  "$DEST_DIR/sdc-mihomo-linux.x86_64"

install -m 0755 \
  "$ROOT_DIR/bin/sdc-mihomo-linux-arm64" \
  "$DEST_DIR/sdc-mihomo-linux.aarch64"

printf 'Copied artifacts to %s\n' "$DEST_DIR"
