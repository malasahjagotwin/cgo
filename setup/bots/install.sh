#!/usr/bin/env bash
set -e

BASE="https://raw.githubusercontent.com/malasahjagotwin/cgo/main"
DIR="${BOT_DIR:-$HOME/bots}"
mkdir -p "$DIR"

if ! command -v node >/dev/null 2>&1; then
  echo "node is required but not found" >&2
  exit 1
fi

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO - "$1"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
}

fetch "$BASE/setup/bots/index.js" > "$DIR/index.js"
cd "$DIR"
exec node index.js