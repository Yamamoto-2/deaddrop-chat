Build output directory for the native CLI clients.

The server embeds this directory (//go:embed all:cli-dist) and serves the
binaries at /cli/bin/<name>. The actual binaries (deaddrop-linux-amd64, etc.)
are build artifacts — generate them with scripts/build-cli.sh (or the Docker
build does it automatically). This placeholder keeps `go build` working before
any binary is built.
