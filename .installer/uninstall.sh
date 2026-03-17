#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="gowatch"
INSTALL_DIR="${GOWATCH_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${HOME}/.gowatch"

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

banner() {
    echo -e "${BOLD}${CYAN}"
    echo "   ____  __        __    _       _     "
    echo "  / ___| \\ \\      / /_ _| |_ ___| |__  "
    echo " | |  _   \\ \\ /\\ / / _\` | __/ __| '_ \\ "
    echo " | |_| |   \\ V  V / (_| | || (__| | | |"
    echo "  \\____|    \\_/\\_/ \\__,_|\\__\\___|_| |_|"
    echo -e "${NC}"
    echo -e "${BOLD}GoWatch Uninstaller${NC}"
    echo ""
}

confirm() {
    local prompt="$1"
    local response

    echo -e -n "${YELLOW}${prompt} [y/N]:${NC} "
    read -r response

    case "$response" in
        [yY][eE][sS]|[yY]) return 0 ;;
        *) return 1 ;;
    esac
}

remove_binary() {
    local binary_path="${INSTALL_DIR}/${BINARY_NAME}"

    if [ ! -f "$binary_path" ]; then
        warn "GoWatch binary not found at $binary_path"
        return 1
    fi

    info "Found GoWatch at: $binary_path"

    if rm "$binary_path" 2>/dev/null; then
        success "Removed binary: $binary_path"
    elif sudo rm "$binary_path"; then
        success "Removed binary: $binary_path"
    else
        error "Failed to remove binary at $binary_path"
        return 1
    fi
}

remove_config() {
    if [ ! -d "$CONFIG_DIR" ]; then
        info "No configuration directory found at $CONFIG_DIR"
        return 0
    fi

    info "Found configuration at: $CONFIG_DIR"

    if confirm "Remove configuration directory ($CONFIG_DIR)?"; then
        rm -rf "$CONFIG_DIR"
        success "Removed configuration: $CONFIG_DIR"
    else
        info "Keeping configuration directory"
    fi
}

main() {
    banner

    if ! confirm "Are you sure you want to uninstall GoWatch?"; then
        info "Uninstall cancelled."
        exit 0
    fi

    echo ""

    local found=false

    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        found=true
    fi

    if [ "$found" = false ]; then
        local search_path
        search_path="$(command -v "$BINARY_NAME" 2>/dev/null || true)"

        if [ -n "$search_path" ]; then
            INSTALL_DIR="$(dirname "$search_path")"
            found=true
            warn "GoWatch not found in $INSTALL_DIR, but found at: $search_path"
        fi
    fi

    if [ "$found" = false ]; then
        error "GoWatch is not installed (not found in $INSTALL_DIR or PATH)"
        exit 1
    fi

    remove_binary
    remove_config

    echo ""
    success "GoWatch has been uninstalled successfully! 👋"
    echo ""
}

main "$@"
