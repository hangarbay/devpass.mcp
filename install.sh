#!/usr/bin/env bash
# Install devpass-usage: downloads the release binary. The binary is both a
# CLI and an MCP stdio server (run `devpass-usage serve`).
#
# One-liner (no clone needed):
#   curl -fsSL https://raw.githubusercontent.com/hangarbay/devpass.mcp/main/install.sh | bash
#
# Inside a clone, --build compiles from source instead of downloading.
set -u

REPO="hangarbay/devpass.mcp"
BIN_DIR="$HOME/.local/bin"
BIN="$BIN_DIR/devpass-usage"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in darwin | linux) ;; *)
  echo "error: unsupported OS '$os'" >&2
  exit 1
  ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *)
    echo "error: unsupported architecture '$arch'" >&2
    exit 1
    ;;
esac

build=0
version=""
arg="${1:-}"
if [ "$arg" = "--build" ]; then
  build=1
  version="${2:-}"
elif [ -n "$arg" ]; then
  version="$arg"
fi
if [ "$build" = 1 ]; then
  echo "==> Building from source"
  command -v go >/dev/null 2>&1 || {
    echo "error: Go is required for --build: https://go.dev/dl" >&2
    exit 1
  }
  mkdir -p "$BIN_DIR"
  go build -o "$BIN" .
  echo "    $BIN (source build)"
else
  echo "==> Downloading release binary"
  if [ -z "$version" ]; then
    version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
      sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
    [ -n "$version" ] || {
      echo "error: could not determine latest release" >&2
      exit 1
    }
  fi
  asset="devpass-usage_${version}_${os}_${arch}"
  base="https://github.com/$REPO/releases/download/$version"
  tmp="$(mktemp -d)" || exit 1
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL -o "$tmp/bin" "$base/$asset" || {
    echo "error: download failed for $asset (check that release $version exists)" >&2
    exit 1
  }
  curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
  expected="$(awk -v a="$asset" '$2 == "*"a || $2 == a {print $1; exit}' "$tmp/checksums.txt")"
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmp/bin" | awk '{print $1}')"
  else
    actual="$(sha256sum "$tmp/bin" | awk '{print $1}')"
  fi
  [ -n "$expected" ] && [ "$actual" = "$expected" ] || {
    echo "error: checksum mismatch for $asset" >&2
    exit 1
  }
  mkdir -p "$BIN_DIR"
  mv "$tmp/bin" "$BIN"
  chmod +x "$BIN"
  echo "    $BIN ($version, ${os}/${arch})"
fi

echo "==> Checking credentials"
if [ -n "${LLM_GATEWAY_SESSION_TOKEN:-}" ]; then
  echo "    using LLM_GATEWAY_SESSION_TOKEN"
elif [ -n "${LLM_GATEWAY_EMAIL:-}" ] && [ -n "${LLM_GATEWAY_PASSWORD:-}" ]; then
  echo "    using LLM_GATEWAY_EMAIL + LLM_GATEWAY_PASSWORD"
else
  cat <<'EOF'
    No DevPass credentials found. Add ONE of the following to your shell
    profile (~/.zshrc, ~/.bashrc, or ~/.crush_env):

      export LLM_GATEWAY_EMAIL="you@example.com"
      export LLM_GATEWAY_PASSWORD="..."

      # or a 30-day session token from the dashboard
      export LLM_GATEWAY_SESSION_TOKEN="..."
EOF
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "note: $BIN_DIR is not on your PATH" ;;
esac

echo "==> Done. Try: devpass-usage show --range 7d"
echo "    For MCP clients: devpass-usage serve  (or use the Docker image)"
