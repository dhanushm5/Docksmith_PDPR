#!/bin/sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

export DOCKSMITH_HOME="$SCRIPT_DIR/.docksmith"

needs_build=0

if [ ! -x ./docksmith ]; then
  needs_build=1
elif ! file ./docksmith | grep -q "ELF"; then
  needs_build=1
elif find . -name '*.go' -newer ./docksmith -print -quit | grep -q .; then
  needs_build=1
fi

if [ "$needs_build" -eq 1 ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "No usable Linux docksmith binary found and Go is not installed." >&2
    echo "Install Go, then run: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o docksmith ." >&2
    exit 1
  fi

  case "$(uname -m)" in
    x86_64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) GOARCH="$(go env GOARCH)" ;;
  esac

  echo "Building Linux docksmith binary (GOARCH=${GOARCH})..."
  GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build -o ./docksmith .
fi

./docksmith "$@"
