#!/usr/bin/env bash
set -euo pipefail

OUT="$HOME/Downloads"

echo "Building broker-trade-sync binaries into $OUT ..."

GOOS=darwin  GOARCH=arm64  go build -o "$OUT/broker-trade-sync-mac-m1"      .
echo "  ✓ broker-trade-sync-mac-m1"

GOOS=darwin  GOARCH=amd64  go build -o "$OUT/broker-trade-sync-mac-intel"   .
echo "  ✓ broker-trade-sync-mac-intel"

GOOS=windows GOARCH=amd64  go build -o "$OUT/broker-trade-sync-windows.exe" .
echo "  ✓ broker-trade-sync-windows.exe"

echo "Done."
