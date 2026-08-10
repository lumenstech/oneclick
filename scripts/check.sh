#!/usr/bin/env sh
set -eu

unformatted="$(gofmt -l cmd internal)"
if [ -n "$unformatted" ]; then
  echo "gofmt required:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go test ./...
go vet ./...
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
CGO_ENABLED=0 go build -trimpath -o "$tmp/oneclick" ./cmd/oneclick
"$tmp/oneclick" version
