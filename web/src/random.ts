// Random generators for names, rooms and passwords. All randomness comes from
// crypto.getRandomValues (available even on insecure origins).

const ADJ = [
  "silent", "hidden", "masked", "nameless", "faceless", "covert",
  "shadow", "ghost", "blind", "sealed",
  "midnight", "dusk", "nocturne", "black", "hollow", "buried",
  "backroom", "underground",
  "fading", "vanishing", "ephemeral", "burning", "ashen", "cinder",
  "last", "oneway",
  "cipher", "encrypted", "secure", "null", "zero", "terminal",
];
const NOUN = [
  "drop", "deadrop", "courier", "mole", "dossier", "cache",
  "stash", "vault", "lockbox", "packet", "capsule",
  "relay", "signal", "wire", "beacon", "channel", "node",
  "socket", "port", "tunnel", "uplink",
  "cipher", "key", "hash", "nonce", "token", "shell",
  "tty", "prompt", "console", "session",
  "ember", "ash", "cinder", "smoke", "trace", "echo",
  "ghost", "afterimage", "burnnote",
];

// base58 alphabet (no ambiguous 0/O/I/l) for readable tokens.
const ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";

function randIndex(n: number): number {
  // Rejection sampling for an unbiased index in [0, n).
  const limit = Math.floor(0x100000000 / n) * n;
  const buf = new Uint32Array(1);
  let x: number;
  do {
    crypto.getRandomValues(buf);
    x = buf[0];
  } while (x >= limit);
  return x % n;
}

function pick<T>(arr: T[]): T {
  return arr[randIndex(arr.length)];
}

function token(len: number): string {
  let out = "";
  for (let i = 0; i < len; i++) out += ALPHABET[randIndex(ALPHABET.length)];
  return out;
}

const cap = (s: string): string => s[0].toUpperCase() + s.slice(1);

// A handle like "SilentRaven".
export function randomNick(): string {
  return cap(pick(ADJ)) + cap(pick(NOUN));
}

// A shareable, unguessable room like "hollow-relay-7Kf3aQ9p". The 8-char token
// adds ~47 bits so the server can't dictionary-attack the routing-id hash.
export function randomRoom(): string {
  return `${pick(ADJ)}-${pick(NOUN)}-${token(8)}`;
}

// A strong password (~117 bits at length 20).
export function randomPassword(len = 20): string {
  return token(len);
}
