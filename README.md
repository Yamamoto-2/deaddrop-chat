# DeadDrop

Anonymous, ephemeral, end-to-end-encrypted chat. The server is a blind
ciphertext relay — a "dead drop": it groups sockets by an opaque routing id and
fans out messages. No accounts, no persistence, no logs.

### ▶ Live demo: **[deaddrop-chat.com](https://deaddrop-chat.com)**

It's a public demo room — anyone can join, so don't send anything sensitive
there. Spin up your own (one binary or `docker run`) for real use.

> Status: **working MVP**. See [`SECURITY.md`](SECURITY.md) for the honest threat model.

## Features

- **End-to-end encrypted** — XChaCha20-Poly1305 with an Argon2id room key, done
  with audited pure-JS crypto (noble), so it works even over plain HTTP, not just
  HTTPS. The server only ever relays ciphertext.
- **Anonymous & ephemeral** — no accounts, no database, no logs; messages live
  only in connected clients. Identity (name + color) is in memory only, never
  persisted — a reload means re-entering your name.
- **Zero-config deploy** — one Go binary, one port, no domain in any config; the
  client derives its WebSocket URL from the page origin.
- **Terminal-style UI** — WebTUI styling, per-user colors, Markdown with code
  highlighting, and encrypted file/image attachments (paste, drag-drop).
- **Native terminal client** — a full-screen TUI delivered fileless via
  `curl <host>/cli | sh`; shares rooms with the web app over the same E2EE.
- **Metadata hardening** — ciphertext length padding, hashed room routing ids,
  and `.onion`-friendly deployment.

## Architecture

- **Backend** — Go, single static binary, embeds the frontend (`go:embed`).
  One port, config via env only, no domain anywhere.
- **Frontend** — vanilla TypeScript + Vite, styled with
  [WebTUI](https://webtui.ironclad.sh) (terminal look, normal app interaction).
- **Routing** — `domain.name/#room` or `domain.name/#room:password`. The fragment never
  reaches the server; it derives an opaque `cyrb53(room)` routing id client-side.
- **Terminal client** — a native Go client, delivered fileless via
  `curl <host>/cli | sh`, speaks the exact same E2EE protocol (see below).

## Develop

Two terminals:

```bash
# 1) backend (relay) on :7337
go run .

# 2) frontend dev server on :5173 (proxies /ws -> :7337)
cd web && npm install && npm run dev
```

Open two browser tabs at `http://localhost:5173/#test` and chat.

## Build (single binary)

```bash
cd web && npm install && npm run build && cd ..
go build -o deaddrop .
./deaddrop          # serves frontend + /ws on :7337
```

## Docker (single container)

```bash
docker build -t deaddrop .
docker run -p 7337:7337 deaddrop
```

Put it behind nginx with `deploy/nginx.conf.example` to serve on port 80
(prefer HTTPS or a Tor `.onion` — the E2EE needs a secure context).

## Terminal client (`curl | sh`)

Built to be **inspected and verified**, not run blindly:

```bash
curl https://<host>/cli                       # 1. read the script first
curl https://<host>/cli | sh                  # 2. interactive: prompts for room/name
curl https://<host>/cli | sh -s -- room:pw n  # preset room:password and nick
```

The script detects OS/arch, downloads a small static binary, and **verifies its
SHA-256** (embedded in the script) before running. Prefer to do it by hand?
Download the binary from `/cli/bin/<os>-<arch>`, check the checksum yourself, and
run it directly — no pipe to `sh` needed. The binary speaks the same WebSocket +
E2EE (Argon2id + XChaCha20-Poly1305 + length padding) as the web app, so CLI and
browser users share rooms transparently. The server never sees plaintext.

As with any `curl | sh`, trust ultimately rests on the host (HTTPS / `.onion`)
and reproducible builds — see [`SECURITY.md`](SECURITY.md).

Binaries are built automatically by `docker build`; for a local `go build`, run
`sh scripts/build-cli.sh` first so the server can embed and serve them.

## Config

| Env | Default | Meaning |
|-----|---------|---------|
| `PORT` | `7337` | Listen port |
| `MAX_FRAME_BYTES` | `10000000` | Max accepted WebSocket frame (~5MB files) |

## Security

E2EE protects message **content** from the server; it does not hide metadata
(connection times, IPs, timing) and, over plain HTTP, can't stop an active
network attacker from tampering with the page code. Prefer **HTTPS** or a Tor
**`.onion`** (a secure context that also hides IPs).

Read [`SECURITY.md`](SECURITY.md) for the full, honest threat model — what
DeadDrop does and does not protect against — before trusting it with anything
sensitive.

## License

[MIT](LICENSE)
