// Wire protocol + URL-fragment helpers.
//
// The fragment (everything after '#') never reaches the server. We split it
// into a room name and an optional password. The server is told only an opaque
// routing id derived from the room name; message bodies (nick/text) are sent
// as-is for now and will become ciphertext once the crypto layer lands.

export interface ParsedHash {
  room: string;
  password: string;
}

export function parseHash(hash: string): ParsedHash | null {
  const raw = hash.startsWith("#") ? hash.slice(1) : hash;
  let dec: string;
  try {
    dec = decodeURIComponent(raw);
  } catch {
    dec = raw;
  }
  if (!dec) return null;
  const idx = dec.indexOf(":");
  if (idx === -1) return { room: dec, password: "" };
  return { room: dec.slice(0, idx), password: dec.slice(idx + 1) };
}

export function buildHash(room: string, password: string): string {
  const body = password ? `${room}:${password}` : room;
  return "#" + encodeURIComponent(body);
}

// Opaque routing id sent to the server: a fast, non-reversible hash of the room
// name. This is only a grouping key, not a secret — message confidentiality
// comes from the separate AES key (the E2EE layer, which uses Web Crypto and
// therefore REQUIRES a secure context: HTTPS or localhost). We deliberately do
// NOT use crypto.subtle here so the routing works even over plain HTTP/IP
// (where SubtleCrypto is unavailable) and is identical across all clients.
export async function deriveRoutingId(room: string): Promise<string> {
  return "r" + cyrb53(room).toString(36);
}

// cyrb53: a small, well-distributed non-cryptographic string hash (53-bit).
function cyrb53(str: string, seed = 0): number {
  let h1 = 0xdeadbeef ^ seed;
  let h2 = 0x41c6ce57 ^ seed;
  for (let i = 0; i < str.length; i++) {
    const ch = str.charCodeAt(i);
    h1 = Math.imul(h1 ^ ch, 2654435761);
    h2 = Math.imul(h2 ^ ch, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507);
  h1 ^= Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507);
  h2 ^= Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return 4294967296 * (2097151 & h2) + (h1 >>> 0);
}

// Wire messages. The server only ever sees {t, room} and an opaque ciphertext
// string `c`. All human content lives inside the encrypted payload below.
export type Outgoing =
  | { t: "join"; room: string }
  | { t: "msg"; room: string; c: string };

export type Incoming =
  | { t: "presence"; room: string; n: number }
  | { t: "msg"; room: string; c: string };

// An attached file, embedded (base64) inside the encrypted payload.
export interface FilePayload {
  name: string;
  mime: string;
  size: number; // original byte length
  data: string; // base64 of the raw bytes
}

// The plaintext that gets encrypted into `c`. Never sent in the clear.
// A chat message carries text, a file, or both. A presence beacon sets `kind`
// ("hello"/"bye") and carries no text/file. `kind` is absent on chat messages,
// so their wire bytes are unchanged (and cross-compatible with the CLI).
export interface MsgPayload {
  nick: string;
  color: string;
  ts: number;
  text?: string;
  file?: FilePayload;
  kind?: "hello" | "bye";
}

// Roster presence: clients announce themselves (encrypted) on join and on a
// heartbeat, and prune peers not heard from within the expiry window. The
// server never sees these — it only relays ciphertext and a raw socket count.
// These constants must match the CLI client (cmd/cli).
export const HELLO_INTERVAL_MS = 15000; // mean heartbeat cadence
export const ROSTER_EXPIRE_MS = 45000;

// Each heartbeat waits a randomized amount rather than a fixed 15s, so the
// encrypted hello beacons don't betray themselves to a traffic observer by
// their metronome-regular timing (real messages are bursty and irregular).
// Bounded to ±50% of the mean, whose max (~22.5s) stays well under the 45s
// expiry so peers are never pruned between beats. NOTE: this only blurs the
// *timing* signal — beacons may still be distinguishable by ciphertext size.
export function nextHelloDelay(): number {
  return HELLO_INTERVAL_MS * (0.5 + Math.random());
}

export type ConnState = "connecting" | "open" | "closed";
