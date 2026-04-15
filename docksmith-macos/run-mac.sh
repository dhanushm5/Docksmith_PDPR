#!/bin/sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

export DOCKSMITH_HOME="$SCRIPT_DIR/.docksmith"

if [ "${1:-}" = "rebuild" ]; then
  echo "Building docksmith Docker image..."
  docker build -t docksmith-dev .
  echo
  echo "Docker image rebuilt."
  exit 0
fi

if [ "${1:-}" = "setup" ]; then
  if ! docker image inspect docksmith-dev >/dev/null 2>&1; then
    echo "Building docksmith Docker image..."
    docker build -t docksmith-dev . >/dev/null
    echo "Docker image rebuilt."
    echo
  fi
  docker run --rm \
    -v "$SCRIPT_DIR/.docksmith:/root/.docksmith" \
    -v "$SCRIPT_DIR:/workspace" \
    -w /workspace \
    docksmith-dev \
    /usr/local/bin/setup-base-image.sh
  exit 0
fi

if [ ! -x ./docksmith ]; then
  make build
fi

./docksmith "$@"
