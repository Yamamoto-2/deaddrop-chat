#!/bin/sh
# Cross-compile the native CLI binaries the server embeds and serves at
# /cli/bin. Run from anywhere; outputs to ./cli-dist.
set -eu
cd "$(dirname "$0")/.."
mkdir -p cli-dist
for target in linux/amd64 linux/arm64; do
  os=${target%/*}
  arch=${target#*/}
  echo "building $os/$arch..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -ldflags="-s -w" -o "cli-dist/deaddrop-$os-$arch" ./cmd/cli
done
echo "done:"
ls -la cli-dist/deaddrop-* 2>/dev/null || true
