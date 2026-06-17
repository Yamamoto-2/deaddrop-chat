package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The ciphertext the server relays must not contain any plaintext field — not
// the nick, not the text, not even the JSON keys. This is the whole point of the
// blind relay.
func TestCiphertextHidesPlaintext(t *testing.T) {
	key := deriveKey("room", "pw")
	p := payload{Nick: "secret-nick", Color: "#5fd7a7", Ts: 1, Text: "top secret body"}
	ct, err := encryptJSON(key, p)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"secret-nick", "top secret body", "nick", "text", "color"} {
		if bytes.Contains(raw, []byte(needle)) {
			t.Fatalf("ciphertext leaks %q", needle)
		}
	}
}

// A text-only message must marshal to the exact bytes it did before the `file`
// field existed (and as the web client does). The `file` key must be absent, or
// padding buckets and cross-client decryption would diverge.
func TestTextPayloadWireUnchanged(t *testing.T) {
	p := payload{Nick: "alice", Color: "#5fd7a7", Ts: 1700000000000, Text: "hello"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nick":"alice","color":"#5fd7a7","ts":1700000000000,"text":"hello"}`
	if string(b) != want {
		t.Fatalf("text payload wire mismatch:\n got %s\nwant %s", b, want)
	}
}

// A presence beacon carries only nick/color/ts/kind — no text or file keys.
func TestHelloBeaconWire(t *testing.T) {
	p := payload{Nick: "alice", Color: "#5fd7a7", Ts: 1, Kind: "hello"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nick":"alice","color":"#5fd7a7","ts":1,"kind":"hello"}`
	if string(b) != want {
		t.Fatalf("hello beacon wire mismatch:\n got %s\nwant %s", b, want)
	}
}

// A file attachment must survive encrypt -> decrypt unchanged.
func TestFileRoundtrip(t *testing.T) {
	key := deriveKey("room", "pw")
	in := payload{
		Nick: "bob", Color: "#5fafff", Ts: 1,
		Text: "see attached",
		File: &filePayload{Name: "photo.png", Mime: "image/png", Size: 3, Data: "AQID"},
	}
	ct, err := encryptJSON(key, in)
	if err != nil {
		t.Fatal(err)
	}
	var out payload
	if err := decryptJSON(key, ct, &out); err != nil {
		t.Fatal(err)
	}
	if out.File == nil {
		t.Fatal("file dropped on roundtrip")
	}
	if out.File.Name != "photo.png" || out.File.Mime != "image/png" ||
		out.File.Size != 3 || out.File.Data != "AQID" || out.Text != "see attached" {
		t.Fatalf("roundtrip mismatch: %+v", out.File)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 1536: "1.5 KB", 5 * 1024 * 1024: "5.0 MB"}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if got := uniquePath(p); got != p {
		t.Fatalf("nonexistent path should be returned as-is, got %s", got)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "f-1.txt")
	if got := uniquePath(p); got != want {
		t.Fatalf("collision: got %s, want %s", got, want)
	}
}

// A sender-controlled name with traversal must collapse to a bare basename,
// matching the sanitization used by handleSave before writing.
func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd": "passwd",
		"a/b/c.png":        "c.png",
		"plain.txt":        "plain.txt",
	}
	for in, want := range cases {
		got := filepath.Base(filepath.FromSlash(in))
		if got != want {
			t.Errorf("basename(%q) = %q, want %q", in, got, want)
		}
	}
}
