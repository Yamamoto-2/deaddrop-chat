# DeadDrop

Anonymous, ephemeral, end-to-end-encrypted chat. The server is a blind
ciphertext relay — a "dead drop": it groups sockets by an opaque routing id and
fans out messages. No accounts, no persistence, no logs.

> Status: **working MVP**. See the [Security](#security) section for the threat model.

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
# 1) backend (relay) on :8080
go run .

# 2) frontend dev server on :5173 (proxies /ws -> :8080)
cd web && npm install && npm run dev
```

Open two browser tabs at `http://localhost:5173/#test` and chat.

## Build (single binary)

```bash
cd web && npm install && npm run build && cd ..
go build -o deaddrop .
./deaddrop          # serves frontend + /ws on :8080
```

## Docker (single container)

```bash
docker build -t deaddrop .
docker run -p 8080:8080 deaddrop
```

Put it behind nginx with `deploy/nginx.conf.example` to serve on port 80
(prefer HTTPS or a Tor `.onion` — the E2EE needs a secure context).

## Terminal client (fileless `curl | sh`)

```bash
curl https://<host>/cli | sh                 # interactive: prompts for room/name
curl https://<host>/cli | sh -s -- room:pw n # preset room:password and nick
curl https://<host>/cli                       # inspect the script first
```

The script detects OS/arch, downloads a tiny static binary into RAM
(`/dev/shm`), verifies its SHA-256, runs it, and deletes it on exit — nothing is
written to persistent disk. The binary speaks the same WebSocket + E2EE
(Argon2id + XChaCha20-Poly1305 + length padding) as the web app, so CLI and
browser users share rooms transparently. The server never sees plaintext.

Binaries are built automatically by `docker build`; for a local `go build`, run
`sh scripts/build-cli.sh` first so the server can embed and serve them.

## Config

| Env | Default | Meaning |
|-----|---------|---------|
| `PORT` | `8080` | Listen port |
| `MAX_FRAME_BYTES` | `10000000` | Max accepted WebSocket frame (~5MB files) |

## Security

E2EE protects message **content** from the server; it does not hide metadata
(connection times, IPs, timing) and, over plain HTTP, can't stop an active
network attacker from tampering with the page code. Prefer **HTTPS** or a Tor
**`.onion`** (a secure context that also hides IPs).

## License

[MIT](LICENSE)
