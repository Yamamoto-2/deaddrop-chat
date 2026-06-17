// End-to-end encryption for a room.
//
// We deliberately do NOT use the browser's Web Crypto (crypto.subtle), because
// it is only exposed in secure contexts (HTTPS/localhost). Using audited pure-JS
// libraries (noble) lets encryption work in ANY context, including plain HTTP.
// Randomness still comes from crypto.getRandomValues, which IS available on
// insecure origins.
//
// Caveat (see README "Security"): over plain HTTP the page code itself is not
// integrity-protected, so an active MITM could swap this file out. HTTP+E2EE
// defeats the honest-but-curious server and passive eavesdroppers, not an active
// on-path attacker. Prefer HTTPS or a Tor .onion for real security.

import { argon2id } from "@noble/hashes/argon2.js";
import { xchacha20poly1305 } from "@noble/ciphers/chacha.js";

// Argon2id parameters. Tunable: higher m/t = slower brute force but slower entry.
const KDF = { t: 2, m: 19456, p: 1, dkLen: 32 } as const;

const enc = new TextEncoder();
const dec = new TextDecoder();

// Derive the 32-byte room key from the room name + optional password. The salt
// is derived from the room name so all clients in a room agree on the key.
export async function deriveKey(
  room: string,
  password: string,
): Promise<Uint8Array> {
  const secret = enc.encode(room + ":" + password);
  const salt = enc.encode("deaddrop|" + room);
  return argon2id(secret, salt, KDF);
}

// Encrypt an object to base64(nonce ‖ ciphertext ‖ tag).
// The plaintext is padded to a size bucket first, so the server can't tell
// messages apart by length (e.g. "yes" vs "no", or which file). Trailing spaces
// (0x20) are valid JSON whitespace, so decrypt just JSON.parses through them.
export function encryptJSON(key: Uint8Array, obj: unknown): string {
  const nonce = crypto.getRandomValues(new Uint8Array(24));
  const json = enc.encode(JSON.stringify(obj));
  const padded = new Uint8Array(padBucket(json.length)).fill(0x20);
  padded.set(json);
  const ct = xchacha20poly1305(key, nonce).encrypt(padded);
  const out = new Uint8Array(nonce.length + ct.length);
  out.set(nonce, 0);
  out.set(ct, nonce.length);
  return bytesToB64(out);
}

// Round a plaintext length up to a bucket: 256-byte steps for normal messages,
// 64KB steps once large (files) so the relative overhead stays tiny.
function padBucket(n: number): number {
  const FINE = 256;
  const COARSE = 64 * 1024;
  if (n <= COARSE) return Math.ceil(n / FINE) * FINE;
  return Math.ceil(n / COARSE) * COARSE;
}

// Decrypt; returns null on any failure (wrong key, tampering, malformed input).
export function decryptJSON<T>(key: Uint8Array, b64: string): T | null {
  try {
    const buf = b64ToBytes(b64);
    const nonce = buf.subarray(0, 24);
    const ct = buf.subarray(24);
    const pt = xchacha20poly1305(key, nonce).decrypt(ct);
    return JSON.parse(dec.decode(pt)) as T;
  } catch {
    return null;
  }
}

// Chunked base64 (handles multi-MB file payloads without per-byte string churn).
export function bytesToB64(bytes: Uint8Array): string {
  let bin = "";
  const CHUNK = 0x8000;
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(bin);
}

export function b64ToBytes(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}
