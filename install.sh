#!/usr/bin/env bash
# Install devpass-usage: downloads the latest release binary (or builds from
# source with --build) and installs the skill for SKILL.md-compatible agents
# (Crush, Claude Code).
set -euo pipefail
cd "$(dirname "$0")"

REPO="hangarbay/devpass-usage.skill"
BIN_DIR="$HOME/.local/bin"
BIN="$BIN_DIR/devpass-usage"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  darwin | linux) ;;
  *) echo "error: unsupported OS '$os' (use --build for source install)" >&2; exit 1 ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

install_skills() {
  echo "==> Installing skills"
  for dest in "$HOME/.config/crush/skills" "$HOME/.claude/skills"; do
    mkdir -p "$dest"
    rm -rf "$dest/devpass-usage"
    cp -R skills/devpass-usage "$dest/devpass-usage"
    echo "    installed -> $dest/devpass-usage"
  done
}

cred_check() {
  echo "==> Checking credentials"
  if [[ -n "${LLM_GATEWAY_SESSION_TOKEN:-}" ]]; then
    echo "    using LLM_GATEWAY_SESSION_TOKEN"
  elif [[ -n "${LLM_GATEWAY_EMAIL:-}" && -n "${LLM_GATEWAY_PASSWORD:-}" ]]; then
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
}

if [[ "${1:-}" == "--build" ]]; then
  echo "==> Building from source"
  command -v go >/dev/null 2>&1 || {
    echo "error: Go is required for --build: https://go.dev/dl" >&2
    exit 1
  }
  mkdir -p "$BIN_DIR"
  go build -o "$BIN" .
else
  echo "==> Downloading release binary"
  version="${1:-}"
  if [[ -z "$version" ]]; then
    version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
      sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
    [[ -n "$version" ]] || {
      echo "error: could not determine latest release (use --build)" >&2
      exit 1
    }
  fi
  asset="devpass-usage_${version}_${os}_${arch}"
  base="https://github.com/$REPO/releases/download/$version"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL -o "$tmp/bin" "$base/$asset"
  curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
  expected="$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$tmp/bin" | awk '{print $1}')"
  else
    actual="$(sha256sum "$tmp/bin" | awk '{print $1}')"
  fi
  [[ -n "$expected" && "$actual" == "$expected" ]] || {
    echo "error: checksum mismatch for $asset" >&2
    exit 1
  }
  mkdir -p "$BIN_DIR"
  mv "$tmp/bin" "$BIN"
  chmod +x "$BIN"
  echo "    $BIN ($version, ${os}/${arch})"
fi

install_skills
cred_check
echo "==> Done. Try: devpass-usage show --range 7d"
