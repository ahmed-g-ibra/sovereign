package client

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

// Scheme is the signature version. It is part of the signed material, so a
// future v2 cannot be downgraded to v1 by rewriting the header alone.
const Scheme = "v1"

const (
	HeaderProbe     = "X-Sovereign-Probe"
	HeaderTimestamp = "X-Sovereign-Timestamp"
	HeaderSignature = "X-Sovereign-Signature"
)

// canonicalString builds the bytes that get signed.
//
// Must stay identical to canonical_string() in api/sovereign_api/signing.py.
// The fixture in sign_test.go and the matching one in the API's
// tests/test_signing.py exist to fail loudly if these two ever drift.
func canonicalString(method, path string, timestamp int64, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		Scheme,
		strings.ToUpper(method),
		path,
		strconv.FormatInt(timestamp, 10),
		hex.EncodeToString(sum[:]),
	}, "\n"))
}

// signRequest returns the value for the signature header.
func signRequest(key ed25519.PrivateKey, method, path string, timestamp int64,
	body []byte) string {
	sig := ed25519.Sign(key, canonicalString(method, path, timestamp, body))
	return Scheme + "=" + base64.StdEncoding.EncodeToString(sig)
}
