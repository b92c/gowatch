#!/usr/bin/env bash
set -euo pipefail

REPO="b92c/gowatch"
BINARY_NAME="gowatch"
INSTALL_DIR="${GOWATCH_INSTALL_DIR:-/usr/local/bin}"
TMP_DIR=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

cleanup() {
    if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

banner() {
    echo -e "${BOLD}${CYAN}"
    echo "   ____  __        __    _       _     "
    echo "  / ___| \\ \\      / /_ _| |_ ___| |__  "
    echo " | |  _   \\ \\ /\\ / / _\` | __/ __| '_ \\ "
    echo " | |_| |   \\ V  V / (_| | || (__| | | |"
    echo "  \\____|    \\_/\\_/ \\__,_|\\__\\___|_| |_|"
    echo -e "${NC}"
    echo -e "${BOLD}GoWatch Installer${NC}"
    echo ""
}

detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        CYGWIN*|MINGW*|MSYS*) echo "windows" ;;
        *)
            error "Unsupported operating system: $os"
            exit 1
            ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

check_dependencies() {
    local missing=()

    if ! command -v curl &>/dev/null; then
        missing+=("curl")
    fi

    if ! command -v sha256sum &>/dev/null && ! command -v shasum &>/dev/null; then
        missing+=("sha256sum or shasum")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        error "Missing required dependencies: ${missing[*]}"
        echo "Please install the missing dependencies and try again."
        exit 1
    fi
}

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "Failed to fetch latest version from GitHub"
        exit 1
    fi

    if ! echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+'; then
        error "Invalid version format: $version"
        exit 1
    fi

    echo "$version"
}

compute_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        error "No SHA256 tool available"
        exit 1
    fi
}

verify_checksum() {
    local file="$1"
    local checksums_file="$2"
    local filename
    filename="$(basename "$file")"

    local expected_hash
    expected_hash=$(grep -F "$filename" "$checksums_file" | awk '{print $1}')

    if [ -z "$expected_hash" ]; then
        error "Checksum not found for $filename in CHECKSUMS.txt"
        exit 1
    fi

    if ! echo "$expected_hash" | grep -qE '^[a-f0-9]{64}$'; then
        error "Invalid checksum format in CHECKSUMS.txt"
        exit 1
    fi

    local actual_hash
    actual_hash=$(compute_sha256 "$file")

    if [ "$actual_hash" != "$expected_hash" ]; then
        error "Checksum verification failed!"
        error "Expected: $expected_hash"
        error "Actual:   $actual_hash"
        exit 1
    fi

    success "Checksum verified"
}

download_file() {
    local url="$1"
    local output="$2"
    local attempt

    for attempt in 1 2 3; do
        if curl -fsSL -o "$output" "$url"; then
            return 0
        fi
        warn "Download attempt $attempt failed, retrying..."
        sleep 2
    done

    error "Failed to download: $url"
    exit 1
}

install_binary() {
    local src="$1"
    local dest="$2"

    if [ ! -d "$dest" ]; then
        info "Creating directory: $dest"
        mkdir -p "$dest" 2>/dev/null || sudo mkdir -p "$dest"
    fi

    local target="${dest}/${BINARY_NAME}"

    if [ -f "$target" ]; then
        warn "GoWatch is already installed at $target"
        warn "Overwriting existing installation..."
    fi

    if cp "$src" "$target" 2>/dev/null; then
        chmod +x "$target"
    elif sudo cp "$src" "$target"; then
        sudo chmod +x "$target"
    else
        error "Failed to install binary to $target"
        error "Try running with sudo or set GOWATCH_INSTALL_DIR to a writable directory"
        exit 1
    fi

    success "Installed GoWatch to $target"
}

main() {
    banner

    local version="${1:-}"

    check_dependencies

    local os arch
    os="$(detect_os)"
    arch="$(detect_arch)"
    info "Detected platform: ${os}/${arch}"

    if [ -z "$version" ]; then
        info "Fetching latest version..."
        version="$(get_latest_version)"
    fi
    info "Installing GoWatch ${version}"

    local ext=""
    if [ "$os" = "windows" ]; then
        ext=".exe"
    fi

    local asset_name="${BINARY_NAME}_${os}_${arch}${ext}"
    local base_url="https://github.com/${REPO}/releases/download/${version}"
    local binary_url="${base_url}/${asset_name}"
    local checksums_url="${base_url}/CHECKSUMS.txt"

    TMP_DIR="$(mktemp -d)"

    info "Downloading ${asset_name}..."
    download_file "$binary_url" "${TMP_DIR}/${asset_name}"

    info "Downloading checksums..."
    download_file "$checksums_url" "${TMP_DIR}/CHECKSUMS.txt"

    info "Verifying integrity..."
    verify_checksum "${TMP_DIR}/${asset_name}" "${TMP_DIR}/CHECKSUMS.txt"

    install_binary "${TMP_DIR}/${asset_name}" "$INSTALL_DIR"

    echo ""
    success "GoWatch ${version} installed successfully! 🎉"
    echo ""
    info "Run ${BOLD}gowatch${NC} to start monitoring your Docker containers."

    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        warn "Directory '$INSTALL_DIR' is not in your PATH."
        warn "Add it to your shell profile:"
        echo ""
        echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
        echo ""
    fi
}

main "$@"
