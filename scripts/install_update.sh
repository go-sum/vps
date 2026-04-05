#!/bin/sh
set -eu

# install_update.sh — Download and install the latest vps release.
# Usage: curl -fsSL https://raw.githubusercontent.com/go-sum/vps/main/scripts/install_update.sh | bash

REPO="go-sum/vps"
# INSTALL_DIR="${HOME}/.local/bin"
INSTALL_DIR="/opt/vps/bin"
LINK_DIR="/usr/local/bin"

main() {
    check_deps

    OS=$(detect_os)
    ARCH=$(detect_arch)
    TAG=$(latest_tag)

    printf "Installing vps %s (%s/%s)\n" "${TAG}" "${OS}" "${ARCH}"

    TARBALL="vps-${TAG}-${OS}-${ARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${TAG}/${TARBALL}"

    TMP=$(mktemp -d)
    trap 'rm -rf "${TMP}"' EXIT

    printf "Downloading %s\n" "${URL}"
    curl -fsSL -o "${TMP}/${TARBALL}" "${URL}"

    mkdir -p "${INSTALL_DIR}"
    tar xzf "${TMP}/${TARBALL}" -C "${INSTALL_DIR}"
    chmod +x "${INSTALL_DIR}/server" "${INSTALL_DIR}/app"

    link_binary "server"
    link_binary "app"

    printf "\nInstalled:\n"
    printf "  %s/server  (%s)\n" "${INSTALL_DIR}" "$("${INSTALL_DIR}/server" --version 2>/dev/null || echo "${TAG}")"
    printf "  %s/app     (%s)\n" "${INSTALL_DIR}" "$("${INSTALL_DIR}/app" --version 2>/dev/null || echo "${TAG}")"
}

check_deps() {
    for cmd in curl tar; do
        if ! command -v "${cmd}" >/dev/null 2>&1; then
            printf "Error: %s is required but not found\n" "${cmd}" >&2
            exit 1
        fi
    done
}

detect_os() {
    case "$(uname -s)" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)
            printf "Error: unsupported OS: %s\n" "$(uname -s)" >&2
            exit 1
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)         echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)
            printf "Error: unsupported architecture: %s\n" "$(uname -m)" >&2
            exit 1
            ;;
    esac
}

latest_tag() {
    tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

    if [ -z "${tag}" ]; then
        printf "Error: could not determine latest release\n" >&2
        exit 1
    fi

    echo "${tag}"
}

link_binary() {
    name="$1"
    src="${INSTALL_DIR}/${name}"
    dst="${LINK_DIR}/${name}"

    if [ -w "${LINK_DIR}" ]; then
        ln -sf "${src}" "${dst}"
    elif command -v sudo >/dev/null 2>&1; then
        sudo ln -sf "${src}" "${dst}"
    else
        printf "Warning: cannot write to %s — add %s to your PATH\n" "${LINK_DIR}" "${INSTALL_DIR}"
        return
    fi

    printf "Linked %s -> %s\n" "${dst}" "${src}"
}

main
