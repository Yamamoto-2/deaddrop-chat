# Security

DeadDrop is an anonymous, ephemeral, end-to-end-encrypted chat relay. This
document is an **honest** description of what it does and does not protect
against. Security software is only as trustworthy as its threat model, so the
limits are stated up front, not buried.

If you are deciding whether to trust DeadDrop with something sensitive, read the
"What it does **not** protect against" section first.

## The one-sentence model

The server is a **blind relay**: it groups WebSocket connections by an opaque
routing id and fans out ciphertext. It never sees plaintext, room names, or
nicknames, and it stores nothing. Everything human-readable is encrypted on the
client with a key derived from the room password, which never leaves the client.

## What the server can and cannot see

| The server sees | The server does **not** see |
|---|---|
| An opaque routing id (`cyrb53(room name)`) | The room name |
| Ciphertext blobs and their (padded) sizes | Message text, nicknames, colors, files |
| Connection metadata: source IP, timing, how many sockets are in each routing id | Who is talking to whom by identity |

There is no database, no history buffer, and no logging of message content. A
client that joins sees only messages sent *after* it connects.

## What it protects against

- **A curious or compromised server operator** reading message content. Keys are
  derived and held only on clients; the server is cryptographically unable to
  read messages.
- **Passive network eavesdroppers** (when used over HTTPS or a Tor `.onion`).
- **After-the-fact forensics / subpoena of stored data.** Nothing is persisted,
  so there is no message history to seize.
- **Distinguishing messages by ciphertext size.** Plaintext is padded to size
  buckets before encryption, so `"yes"` and `"no"` are indistinguishable on the
  wire.

## What it does **not** protect against

These are real limitations. Do not rely on DeadDrop beyond them.

- **Metadata.** The server (and anyone who can observe the network) can see
  connection times, IP addresses, message timing, which routing ids are active,
  and how many sockets are in each room. Hiding metadata fully is a different and
  much larger problem. Use Tor (`.onion`) if metadata matters.
- **Web client code integrity over plain HTTP.** With browser clients, the
  JavaScript that performs the encryption is delivered by the server on each
  visit. This is effectively *trust on first use, every use*: a malicious or
  compromised server — or an **active** on-path attacker over plain HTTP — can
  serve modified code that exfiltrates the key. HTTP + E2EE defeats a passive
  eavesdropper and an honest-but-curious server; it does **not** defeat an active
  MITM. **Prefer HTTPS or a Tor `.onion`** (a secure context whose code is
  authenticated).
- **Malicious room members.** Anyone with the room link/password is a trusted
  participant by design (there are no per-user identities). Members can screenshot
  or log everything. DeadDrop secures the transport, not the people in the room.
- **No forward secrecy.** The room key is static (derived from room + password),
  so a key compromise would expose any ciphertext an attacker captured live.
  Mitigation: because nothing is stored, there is no archive of past ciphertext
  to decrypt later — only live capture has exposure.
- **Weak room privacy without a password.** The routing id is a fast,
  non-cryptographic hash (`cyrb53`) of the room name, so common room names are
  guessable by dictionary attack. A guessed room name reveals *who is connected*,
  but **with a password the content stays secure regardless**. For unlisted
  rooms, use the random-room button (high-entropy name) and a password.
- **Endpoint compromise.** Malware, a keylogger, or a hostile OS on either end
  defeats any chat encryption. Out of scope.

## Cryptography

- **Key derivation:** `Argon2id` over `roomName + ":" + password` with salt
  `"deaddrop|" + roomName` (parameters: t=2, m=19456 KiB, p=1, 32-byte output).
  The password never leaves the client and is never folded into the routing id.
- **Cipher:** `XChaCha20-Poly1305` (AEAD) with a fresh random 24-byte nonce per
  message. Wire ciphertext is `base64(nonce ‖ ciphertext ‖ tag)`.
- **Padding:** plaintext is padded to size buckets before encryption — 256-byte
  steps up to 64 KiB, then 64 KiB steps — so ciphertext length leaks little.
- **Routing id:** `"r" + base36(cyrb53(roomName))`. This is a grouping key only,
  **not** a secret and not cryptographic.
- **Libraries:** the web client uses audited pure-JS crypto
  (`@noble/hashes`, `@noble/ciphers`) rather than the Web Crypto API, so
  encryption works in any context including plain HTTP. Randomness still comes
  from `crypto.getRandomValues`. The Go terminal client implements the identical
  scheme, byte-for-byte, so web and CLI users share rooms.
- **Presence/roster:** the member list is built from *encrypted* presence beacons
  exchanged between clients. The server still only sees a socket count; nicknames
  in the roster are visible to room members (intended), never to the server.

## Terminal client (`curl | sh`)

The terminal client is served at `GET /cli` as a small shell bootstrap. It is
designed to be **inspected and verified**, not run blindly:

- **Inspect it first:** `curl https://<host>/cli` prints the script. Read it.
- **It verifies integrity:** the script checks the downloaded binary's SHA-256
  against a value embedded in the script before running it.
- **You can do it by hand:** download the binary from `/cli/bin/<os>-<arch>`,
  verify its SHA-256 yourself, and run it manually — no pipe to `sh` required.

The same root-of-trust caveat applies as for the web client: the script and
binary are served by the host, so trust ultimately rests on HTTPS / `.onion` and
(ideally) reproducible builds. Treat `curl | sh` from an untrusted host the way
you would any other remote code.

## Deployment recommendations

- **Serve over HTTPS**, or as a **Tor `.onion`** service. A `.onion` is
  self-authenticating, counts as a secure context, and also hides server/client
  IPs — the strongest option for metadata protection. See
  `deploy/nginx.conf.example`.
- Do not serve the app on plain HTTP in production beyond local testing.
- The relay holds no shared state, so it does not scale horizontally without a
  pub/sub layer — fine for ephemeral rooms, out of scope otherwise.

## Reporting a vulnerability

Please report security issues privately rather than opening a public issue.
Use GitHub's **Report a vulnerability** (Security Advisories) on the repository,
or open a minimal issue asking for a private contact channel. Include steps to
reproduce, affected version/commit, and impact. We aim to acknowledge reports
promptly and will credit reporters who wish to be credited.
