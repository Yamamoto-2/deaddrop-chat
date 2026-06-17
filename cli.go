package main

// Serves the native CLI client: `GET /cli` returns a fileless bootstrap script
// (curl | sh), and `GET /cli/bin/<name>` serves the embedded static binaries.
// The script and download URLs are derived from the request host, so there is
// still no domain anywhere in the config.

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
)

// Built by scripts/build-cli (and the Docker web/cli stage) into cli-dist/.
// The placeholder keeps `go build` working before any binary is built.
//
//go:embed all:cli-dist
var cliFS embed.FS

// filename -> sha256 hex, computed once at startup.
var cliHashes = map[string]string{}

func init() {
	entries, err := fs.ReadDir(cliFS, "cli-dist")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "deaddrop-") {
			continue
		}
		b, err := cliFS.ReadFile("cli-dist/" + e.Name())
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		cliHashes[e.Name()] = hex.EncodeToString(sum[:])
	}
}

func reqScheme(r *http.Request) string {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func cliScriptHandler(w http.ResponseWriter, r *http.Request) {
	scheme := reqScheme(r)
	base := scheme + "://" + r.Host
	wsScheme := "ws"
	if scheme == "https" {
		wsScheme = "wss"
	}
	wss := wsScheme + "://" + r.Host

	// One `os-arch) sha=...` case per embedded binary.
	names := make([]string, 0, len(cliHashes))
	for name := range cliHashes {
		names = append(names, name)
	}
	sort.Strings(names)
	var cases strings.Builder
	for _, name := range names {
		osArch := strings.TrimPrefix(name, "deaddrop-")
		fmt.Fprintf(&cases, "    %s) sha=%q ;;\n", osArch, cliHashes[name])
	}

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, cliScript, base, cases.String(), wss)
}

func cliBinHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/cli/bin/")
	if _, ok := cliHashes[name]; !ok {
		http.Error(w, "no such CLI binary (build cli-dist first)", http.StatusNotFound)
		return
	}
	b, err := cliFS.ReadFile("cli-dist/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(b)
}

// %[1]s = base url, %[2]s = arch->sha cases, %[3]s = ws base url.
const cliScript = `#!/bin/sh
# DeadDrop terminal client — bootstrap script. Made to be inspected and verified,
# not run blindly: it downloads a small static binary, checks its SHA-256 against
# the value embedded below, and only then runs it. The binary speaks the same
# end-to-end-encrypted protocol as the web app; the server never sees plaintext.
#
# Inspect this script:        curl %[1]s/cli
# Run it:                     curl %[1]s/cli | sh
# With a room preset:         curl %[1]s/cli | sh -s -- room:password nick
# Or verify by hand:          download %[1]s/cli/bin/<os>-<arch>, check sha256, run
set -eu

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "deaddrop: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "deaddrop: unsupported arch $(uname -m)" >&2; exit 1 ;;
esac

sha=""
case "$os-$arch" in
%[2]s    *) echo "deaddrop: no prebuilt binary for $os-$arch" >&2; exit 1 ;;
esac

bin="deaddrop-$os-$arch"
dir="${XDG_RUNTIME_DIR:-/dev/shm}"
{ [ -d "$dir" ] && [ -w "$dir" ]; } || dir="$(mktemp -d)"
f="$(mktemp "$dir/.dd.XXXXXX")"
trap 'rm -f "$f"' EXIT INT TERM

curl -fsSL "%[1]s/cli/bin/$bin" -o "$f"
if command -v sha256sum >/dev/null 2>&1; then
  echo "$sha  $f" | sha256sum -c - >/dev/null 2>&1 || { echo "deaddrop: checksum mismatch, aborting" >&2; exit 1; }
fi
chmod +x "$f"

# </dev/tty restores the keyboard as stdin (the pipe occupied it).
DD_SERVER="%[3]s/ws" "$f" "$@" </dev/tty
`
