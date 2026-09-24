#!/usr/bin/env sh

set -eu

REPOSITORY="benoitpetit/mira"
VERSION="${MIRA_VERSION:-latest}"
INSTALL_DIR="${MIRA_INSTALL_DIR:-${HOME}/.local/bin}"
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t mira-install)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

fail() {
  printf '%s\n' "MIRA install error: $*" >&2
  exit 1
}

download() {
  url="$1"
  destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
  else
    fail "curl or wget is required"
  fi
}

OS="$(uname -s)"
MACHINE="$(uname -m)"
case "$OS" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *) fail "unsupported operating system: $OS (supported: Linux, macOS)" ;;
esac

case "$MACHINE" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "unsupported architecture: $MACHINE (supported: amd64, arm64)" ;;
esac

ASSET="mira-${OS}-${ARCH}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_ROOT="https://github.com/${REPOSITORY}/releases/latest/download"
else
  DOWNLOAD_ROOT="https://github.com/${REPOSITORY}/releases/download/v${VERSION}"
fi

ARCHIVE="${TMP_DIR}/${ASSET}"
CHECKSUMS="${TMP_DIR}/SHA256SUMS"
download "${DOWNLOAD_ROOT}/${ASSET}" "$ARCHIVE" || fail "could not download ${ASSET}"
download "${DOWNLOAD_ROOT}/SHA256SUMS" "$CHECKSUMS" || fail "could not download SHA256SUMS"

EXPECTED="$(awk -v asset="$ASSET" '$2 == asset { print $1; exit }' "$CHECKSUMS")"
[ -n "$EXPECTED" ] || fail "SHA256SUMS does not contain ${ASSET}"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$ARCHIVE" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$ARCHIVE" | awk '{ print $1 }')"
else
  fail "sha256sum or shasum is required to verify the download"
fi
[ "$EXPECTED" = "$ACTUAL" ] || fail "checksum verification failed for ${ASSET}"

mkdir -p "$TMP_DIR/extracted" "$INSTALL_DIR"
tar -xzf "$ARCHIVE" -C "$TMP_DIR/extracted"
[ -f "$TMP_DIR/extracted/mira" ] || fail "archive does not contain the mira binary"
install -m 0755 "$TMP_DIR/extracted/mira" "${INSTALL_DIR}/mira"

printf 'MIRA installed at %s/mira\n' "$INSTALL_DIR"
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) printf 'Add %s to your PATH to run: mira --version\n' "$INSTALL_DIR" ;;
esac
