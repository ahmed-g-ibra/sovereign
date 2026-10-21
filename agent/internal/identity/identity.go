// Package identity stores a probe's credentials on disk.
//
// The private key is generated on the probe and never leaves it. The server
// knows only the public half, so a compromise of the control plane cannot
// forge measurements.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const fileName = "identity.json"

// Identity is the probe's half of the trust relationship.
type Identity struct {
	ProbeID    string `json:"probe_id"`
	PrivateKey string `json:"private_key"` // base64, Ed25519 seed
	Server     string `json:"server"`
}

// Generate creates a new keypair, returning the identity and the base64 public
// key to send to the server.
func Generate(server string) (*Identity, string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}
	// Store the 32-byte seed rather than the 64-byte expanded key: it is what
	// the Python side's Ed25519PrivateKey.from_private_bytes expects, and the
	// full key is derivable from it.
	id := &Identity{
		PrivateKey: base64.StdEncoding.EncodeToString(priv.Seed()),
		Server:     server,
	}
	return id, base64.StdEncoding.EncodeToString(pub), nil
}

// Signer returns the Ed25519 key for signing requests.
func (i *Identity) Signer() (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(i.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("private key is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// Save writes the identity with owner-only permissions.
func (i *Identity) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fileName)

	// Write-then-rename: an interrupted save leaves the previous identity
	// intact rather than a truncated file that locks the probe out.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a stored identity.
func Load(dir string) (*Identity, error) {
	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return nil, err
	}
	var id Identity
	if err := json.Unmarshal(b, &id); err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	if id.ProbeID == "" || id.PrivateKey == "" {
		return nil, fmt.Errorf("identity file is incomplete")
	}
	return &id, nil
}

// Exists reports whether this probe has already enrolled.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, fileName))
	return err == nil
}
