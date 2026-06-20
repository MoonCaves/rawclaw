#!/bin/sh
# rawclaw installer — one command, no toolchain required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/MoonCaves/rawclaw/main/install.sh | sh
#   — or, from a checkout —
#   ./install.sh
#
# What it does:
#   1. Detects your OS + CPU and downloads the matching prebuilt release binary.
#   2. Installs `rawclaw` to ~/.local/bin (warns if that's not on your PATH).
#   3. Installs the Claude Code skill to ~/.claude/skills/rawclaw-search/.
#
# No Go toolchain, no compiler, no root, no sudo.

set -eu
# Enable pipefail where the shell supports it (bash/zsh/ksh); ignore on plain dash.
# shellcheck disable=SC3040
(set -o pipefail) 2>/dev/null && set -o pipefail || true

REPO="MoonCaves/rawclaw"
BIN_NAME="rawclaw"
SKILL_NAME="rawclaw-search"

# ── Output helpers (colour only on a real terminal) ───────────────────────────
if [ -t 1 ] && command -v tput >/dev/null 2>&1 && [ -n "$(tput colors 2>/dev/null || echo)" ]; then
    BOLD="$(tput bold)"; GREEN="$(tput setaf 2)"; YELLOW="$(tput setaf 3)"
    CYAN="$(tput setaf 6)"; RESET="$(tput sgr0)"
else
    BOLD=""; GREEN=""; YELLOW=""; CYAN=""; RESET=""
fi

info() { printf '%s==>%s %s\n' "${CYAN}${BOLD}" "${RESET}" "$*"; }
ok()   { printf '%s ok %s %s\n' "${GREEN}${BOLD}" "${RESET}" "$*"; }
warn() { printf '%s !  %s %s\n' "${YELLOW}${BOLD}" "${RESET}" "$*" >&2; }
die()  { printf '\n%sError:%s %s\n' "${BOLD}" "${RESET}" "$*" >&2; exit 1; }

# ── Require a downloader ──────────────────────────────────────────────────────
command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 \
    || die "need either curl or wget installed to download rawclaw."

# fetch <url> <dest> — download a URL to a file, failing loudly.
fetch() {
    _url="$1"; _dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$_url" -o "$_dest" || die "download failed: $_url"
    else
        wget -qO "$_dest" "$_url" || die "download failed: $_url"
    fi
}

# ── Detect OS + ARCH (GoReleaser naming) ──────────────────────────────────────
info "Detecting platform..."
OS="$(uname -s 2>/dev/null || echo unknown)"
case "$OS" in
    Linux)  OS="linux"  ;;
    Darwin) OS="darwin" ;;
    *) die "unsupported OS '$OS'. rawclaw ships prebuilt binaries for linux and darwin only." ;;
esac

ARCH="$(uname -m 2>/dev/null || echo unknown)"
case "$ARCH" in
    x86_64 | amd64)          ARCH="amd64" ;;
    arm64  | aarch64)        ARCH="arm64" ;;
    *) die "unsupported CPU architecture '$ARCH'. rawclaw ships amd64 and arm64 builds." ;;
esac
ok "Platform: ${OS}/${ARCH}"

# ── Download + extract the release tarball ────────────────────────────────────
ASSET="${BIN_NAME}_${OS}_${ARCH}.tar.gz"
ASSET_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

TMPDIR_RC="$(mktemp -d 2>/dev/null || mktemp -d -t rawclaw)" \
    || die "could not create a temp directory."
# Clean up the temp dir on any exit.
trap 'rm -rf "$TMPDIR_RC"' EXIT INT TERM

info "Downloading ${ASSET}..."
fetch "$ASSET_URL" "$TMPDIR_RC/$ASSET"

info "Extracting..."
tar -xzf "$TMPDIR_RC/$ASSET" -C "$TMPDIR_RC" \
    || die "could not extract $ASSET (corrupt download?)."

# GoReleaser puts the binary at the archive root; guard against a nested layout.
SRC_BIN="$TMPDIR_RC/$BIN_NAME"
if [ ! -f "$SRC_BIN" ]; then
    SRC_BIN="$(find "$TMPDIR_RC" -type f -name "$BIN_NAME" 2>/dev/null | head -n 1)"
fi
[ -n "${SRC_BIN:-}" ] && [ -f "$SRC_BIN" ] \
    || die "the '$BIN_NAME' binary was not found inside $ASSET."

# ── Install the binary into ~/.local/bin ──────────────────────────────────────
BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR" || die "could not create $BIN_DIR."

DEST_BIN="$BIN_DIR/$BIN_NAME"
# Install via a temp name + mv so a running rawclaw isn't clobbered mid-write.
chmod +x "$SRC_BIN" 2>/dev/null || true
cp "$SRC_BIN" "$DEST_BIN.new" || die "could not write to $BIN_DIR."
chmod +x "$DEST_BIN.new"
mv -f "$DEST_BIN.new" "$DEST_BIN" || die "could not install binary to $DEST_BIN."
ok "Installed $BIN_NAME -> $DEST_BIN"

# Warn (don't fail) if ~/.local/bin isn't on PATH.
case ":${PATH:-}:" in
    *":$BIN_DIR:"*) : ;;
    *)
        warn "$BIN_DIR is not on your PATH."
        warn "Add this to your shell profile (~/.bashrc, ~/.zshrc, ~/.profile):"
        warn "    export PATH=\"\$HOME/.local/bin:\$PATH\""
        ;;
esac

# ── Install the Claude Code skill ─────────────────────────────────────────────
# Honour CLAUDE_CONFIG_DIR if set, else the default ~/.claude.
CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
SKILL_DEST="$CLAUDE_DIR/skills/$SKILL_NAME"
SKILL_REL="skills/$SKILL_NAME/SKILL.md"

# When run from a checkout, copy the local SKILL.md; otherwise fetch the raw file.
SCRIPT_SKILL=""
if [ -n "${0:-}" ] && [ "$0" != "sh" ] && [ "$0" != "-sh" ] && [ "$0" != "bash" ]; then
    _script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" 2>/dev/null && pwd -P || echo)"
    if [ -n "$_script_dir" ] && [ -f "$_script_dir/$SKILL_REL" ]; then
        SCRIPT_SKILL="$_script_dir/$SKILL_REL"
    fi
fi

info "Installing Claude Code skill -> $SKILL_DEST"
mkdir -p "$SKILL_DEST" || die "could not create $SKILL_DEST."

if [ -n "$SCRIPT_SKILL" ]; then
    cp "$SCRIPT_SKILL" "$SKILL_DEST/SKILL.md" || die "could not copy SKILL.md."
else
    SKILL_URL="https://raw.githubusercontent.com/${REPO}/main/$SKILL_REL"
    fetch "$SKILL_URL" "$SKILL_DEST/SKILL.md"
fi
ok "Skill installed (use it from any Claude Code session)."

# ── Done ──────────────────────────────────────────────────────────────────────
printf '\n%s%srawclaw is ready.%s\n\n' "${BOLD}" "${GREEN}" "${RESET}"
printf '  Try it:\n'
printf '    %s "where did we set up auth"\n' "$BIN_NAME"
printf '    %s --list\n' "$BIN_NAME"
printf '    %s --help\n' "$BIN_NAME"
printf '\n'
printf '  Docs: https://github.com/%s\n\n' "$REPO"
