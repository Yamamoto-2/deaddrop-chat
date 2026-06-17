# DeadDrop wire protocol

This is a stable description of the protocol so you can build your own client or
bot. The reference implementations are the web client (`web/src/`) and the Go
terminal client (`cmd/cli/`); they are byte-for-byte compatible and either can
read the other's messages.

The guiding rule: **the server only ever sees `{t, room, c, n}` and an opaque
ciphertext `c`.** Everything human-readable (nicknames, text, files, presence)
lives inside the encrypted payload. A correct client does all crypto locally.

## 1. Transport

- A single WebSocket endpoint: `GET /ws` (text frames, JSON).
- The client derives the WebSocket URL from the page origin:
  `("wss" if https else "ws") + "://" + host + "/ws"`.
- The server applies a max frame size (`MAX_FRAME_BYTES`, default 10,000,000).
  Frames larger than this are rejected and the connection is closed.

## 2. The URL fragment (never sent to the server)

Rooms are addressed by a URL fragment, which browsers never transmit:

```
https://host/#room               room, no password (weak privacy)
https://host/#room:password      room + password (strong E2EE)
```

Parsing: strip a leading `#`, `decodeURIComponent`, then split on the **first**
`:` into `room` and `password` (password may be empty). The terminal client
takes the same `room:password` as its first CLI argument.

## 3. Local derivation

From `room` (and optional `password`) the client computes two values:

### Routing id (sent to the server, not secret)

```
routingId = "r" + base36( cyrb53(room) )
```

`cyrb53` is a fast, non-cryptographic 53-bit string hash. It iterates UTF-16
code units (JS `charCodeAt`) with 32-bit `Math.imul` mixing; the Go client
reproduces this exactly over `utf16.Encode([]rune(room))`. This is only a
grouping key — it is **not** a secret, and common room names are guessable.
**The password is never folded into the routing id** (doing so would let the
server brute-force it offline).

### Room key (never leaves the client)

```
secret = room + ":" + password
salt   = "deaddrop|" + room
key    = Argon2id(secret, salt, t=2, m=19456 KiB, p=1, outputLen=32)   // 32 bytes
```

Memory `m` is in KiB on both sides (`x/crypto/argon2` and `@noble/hashes`), so
`19456` matches. With no password, the key is derived from the room name alone —
documented as weak (anyone who knows the room name can decrypt).

## 4. Message envelope (client ⇄ server)

All frames are JSON objects. The server reads only `t` and `room`; it forwards
`c` verbatim and never inspects it.

**Client → server**

```jsonc
{ "t": "join", "room": "<routingId>" }              // join a room
{ "t": "msg",  "room": "<routingId>", "c": "<b64>" } // send ciphertext
```

**Server → client**

```jsonc
{ "t": "presence", "room": "<routingId>", "n": <int> }  // socket count changed
{ "t": "msg",      "room": "<routingId>", "c": "<b64>" } // relayed ciphertext
```

Routing rules:
- `join` adds the socket to the room and triggers a `presence` broadcast.
- `msg` is fanned out to **every other** socket in the same room (no self-echo,
  no history buffer — a client only sees messages sent after it joined).
- A `msg` is only relayed if the sender has joined that exact `room`.
- On disconnect the socket leaves its room and a final `presence` is broadcast.
- `n` is a raw socket count. It carries **no identities** — names come from the
  encrypted presence beacons in §6.

## 5. Encrypted payload (the contents of `c`)

`c` is produced as follows:

```
plaintext = JSON.stringify(payload)                 // UTF-8
padded    = plaintext padded with spaces (0x20) up to padBucket(len)
sealed    = XChaCha20-Poly1305.seal(key, nonce, padded)   // nonce: 24 random bytes
c         = base64_std( nonce ‖ sealed )            // sealed = ciphertext ‖ 16-byte tag
```

Decryption reverses this: base64-decode, split the first 24 bytes as the nonce,
`open`, then `JSON.parse` (trailing pad spaces are ignored by JSON parsers).

### Padding buckets

```
padBucket(n):
  fine   = 256
  coarse = 65536            // 64 KiB
  if n <= coarse:  return ceil(n / fine)   * fine
  else:            return ceil(n / coarse) * coarse
```

So small messages round up to 256-byte steps and large ones to 64 KiB steps,
hiding exact lengths.

### Payload object

```jsonc
{
  "nick":  "string",          // display name
  "color": "#rrggbb",         // display color
  "ts":    1700000000000,     // client epoch ms
  "text":  "string",          // optional: chat text (markdown)
  "file":  { ... },           // optional: attachment (see below)
  "kind":  "hello" | "bye"    // optional: presence beacon (see §6)
}
```

- A **chat message** sets `text`, `file`, or both, and omits `kind`. Encoders
  MUST omit absent fields (`text`/`file`/`kind`) so chat-message bytes are stable
  and identical across clients.
- A **file** object:
  ```jsonc
  { "name": "photo.png", "mime": "image/png", "size": 12345, "data": "<base64>" }
  ```
  `data` is standard base64 (`btoa`/`atob` == Go `base64.StdEncoding`) of the raw
  bytes. Reference clients cap raw file size at 5 MiB so the encrypted frame
  stays under `MAX_FRAME_BYTES`.

Text is rendered as Markdown by the reference clients; treat incoming text as
untrusted and sanitize before rendering as HTML.

## 6. Presence / roster beacons

The server reports only a socket count, so member **names** are exchanged
client-to-client as encrypted beacons — ordinary `{t:"msg"}` frames whose
payload sets `kind`:

```jsonc
{ "nick": "...", "color": "...", "ts": ..., "kind": "hello" }  // I'm here
{ "nick": "...", "color": "...", "ts": ..., "kind": "bye" }    // I'm leaving
```

Reference behaviour:
- Send `hello` on join and every `HELLO_INTERVAL_MS` (15 s) as a heartbeat.
- On receiving a `hello` from a peer you didn't know, reply with your own `hello`
  so newcomers learn existing members immediately.
- Drop peers you haven't heard from in `ROSTER_EXPIRE_MS` (45 s).
- Send `bye` on leave (best-effort; expiry covers ungraceful exits).

A client may ignore beacons entirely (a minimal bot only needs §4–§5); it just
won't track a roster. Beacons are never rendered as chat.

## 7. Minimal client checklist

1. Parse `room[:password]` from the fragment / CLI arg.
2. Compute `routingId` (§3) and `key` (§3).
3. Open `/ws`, send `{t:"join", room: routingId}`.
4. To send: build a payload (§5), encrypt to `c`, send `{t:"msg", room, c}`.
5. To receive: on `{t:"msg"}`, decrypt `c`; branch on `kind` (chat vs beacon).
6. Optionally implement §6 to show who's online.
