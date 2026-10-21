package client

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

// Known-answer test shared with the API's tests/test_signing.py.
//
// If someone edits the canonical string on one side only, exactly one of these
// two tests fails. That is the whole point of duplicating the vector.
const (
	fixtureTimestamp = int64(1761000000)
	fixturePath      = "/v1/samples"
	fixtureBody      = `{"hello":"gulf"}`
	fixtureSignature = "v1=0wNien+Fk3IJTqkQYdRioglwjDdtftdZqab+leID/sMgk+tYj6kYwaZXMt/YP/GWqiE9lwlByykDxjwgN/EOCQ=="
)

func fixtureKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = 1
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestCrossLanguageFixture(t *testing.T) {
	got := signRequest(fixtureKey(), "POST", fixturePath, fixtureTimestamp,
		[]byte(fixtureBody))
	if got != fixtureSignature {
		t.Fatalf("signature drift between Go and Python.\n got: %s\nwant: %s", got,
			fixtureSignature)
	}
}

func TestCanonicalStringShape(t *testing.T) {
	lines := strings.Split(string(canonicalString("post", fixturePath,
		fixtureTimestamp, []byte(fixtureBody))), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}
	if lines[0] != Scheme {
		t.Errorf("line 0 = %q, want %q", lines[0], Scheme)
	}
	if lines[1] != "POST" {
		t.Errorf("method not upper-cased: %q", lines[1])
	}
	if lines[2] != fixturePath {
		t.Errorf("line 2 = %q, want %q", lines[2], fixturePath)
	}
}

func TestEverySignedFieldChangesTheSignature(t *testing.T) {
	key := fixtureKey()
	base := signRequest(key, "POST", fixturePath, fixtureTimestamp, []byte(fixtureBody))

	variants := map[string]string{
		"method":    signRequest(key, "GET", fixturePath, fixtureTimestamp, []byte(fixtureBody)),
		"path":      signRequest(key, "POST", "/v1/config", fixtureTimestamp, []byte(fixtureBody)),
		"timestamp": signRequest(key, "POST", fixturePath, fixtureTimestamp+1, []byte(fixtureBody)),
		"body":      signRequest(key, "POST", fixturePath, fixtureTimestamp, []byte(fixtureBody+" ")),
	}
	for name, got := range variants {
		if got == base {
			t.Errorf("changing the %s did not change the signature", name)
		}
	}
}

func TestSignatureIsVerifiableWithThePublicKeyAlone(t *testing.T) {
	key := fixtureKey()
	sig := signRequest(key, "POST", fixturePath, fixtureTimestamp, []byte(fixtureBody))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sig, Scheme+"="))
	if err != nil {
		t.Fatal(err)
	}
	pub := key.Public().(ed25519.PublicKey)
	if !ed25519.Verify(pub, canonicalString("POST", fixturePath, fixtureTimestamp,
		[]byte(fixtureBody)), raw) {
		t.Fatal("public key failed to verify a signature it should accept")
	}
}
