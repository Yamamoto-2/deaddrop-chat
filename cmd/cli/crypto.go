package main

// E2EE that is byte-for-byte compatible with the web client (web/src/crypto.ts,
// proto.ts, random.ts). Any divergence here means the CLI and browser can't read
// each other, so the constants below must match exactly.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf16"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Wire envelope (matches proto.ts). The server only ever sees t/room/c/n.
type envelope struct {
	T    string `json:"t"`
	Room string `json:"room"`
	C    string `json:"c,omitempty"`
	N    int    `json:"n,omitempty"`
}

// Encrypted message body (matches MsgPayload). Key order is irrelevant — the
// receiver JSON-decodes regardless of field order.
type payload struct {
	Nick  string `json:"nick"`
	Color string `json:"color"`
	Ts    int64  `json:"ts"`
	Text  string `json:"text,omitempty"`
}

// routingID == web deriveRoutingId: "r" + cyrb53(room).toString(36).
func routingID(room string) string {
	return "r" + strconv.FormatUint(cyrb53(room), 36)
}

// cyrb53 reproduces the JS hash exactly. JS uses charCodeAt (UTF-16 code units)
// and Math.imul (32-bit multiply), so we iterate UTF-16 units and keep all of
// the mixing in uint32.
func cyrb53(s string) uint64 {
	var h1 uint32 = 0xdeadbeef
	var h2 uint32 = 0x41c6ce57
	for _, u := range utf16.Encode([]rune(s)) {
		ch := uint32(u)
		h1 = (h1 ^ ch) * 2654435761
		h2 = (h2 ^ ch) * 1597334677
	}
	h1 = (h1 ^ (h1 >> 16)) * 2246822507
	h1 ^= (h2 ^ (h2 >> 13)) * 3266489909
	h2 = (h2 ^ (h2 >> 16)) * 2246822507
	h2 ^= (h1 ^ (h1 >> 13)) * 3266489909
	return 4294967296*uint64(h2&2097151) + uint64(h1)
}

// deriveKey == web deriveKey: Argon2id(room+":"+pass, "deaddrop|"+room).
// Memory is in KiB for both x/crypto/argon2 and noble, so 19456 matches.
func deriveKey(room, password string) []byte {
	secret := []byte(room + ":" + password)
	salt := []byte("deaddrop|" + room)
	return argon2.IDKey(secret, salt, 2, 19456, 1, 32)
}

// padBucket == web padBucket: 256-byte steps up to 64KiB, then 64KiB steps.
func padBucket(n int) int {
	const fine = 256
	const coarse = 64 * 1024
	if n <= coarse {
		return (n + fine - 1) / fine * fine
	}
	return (n + coarse - 1) / coarse * coarse
}

// encryptJSON == web encryptJSON: pad plaintext with spaces to a bucket, then
// XChaCha20-Poly1305, then base64(nonce ‖ ct ‖ tag).
func encryptJSON(key []byte, v any) (string, error) {
	js, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	padded := make([]byte, padBucket(len(js)))
	for i := range padded {
		padded[i] = ' '
	}
	copy(padded, js)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, padded, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// decryptJSON reverses encryptJSON. json.Unmarshal tolerates the trailing pad
// spaces, just like JSON.parse on the web.
func decryptJSON(key []byte, b64 string, v any) error {
	buf, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	if len(buf) < chacha20poly1305.NonceSizeX {
		return errors.New("ciphertext too short")
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}
	nonce := buf[:chacha20poly1305.NonceSizeX]
	ct := buf[chacha20poly1305.NonceSizeX:]
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(pt, v)
}
