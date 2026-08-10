package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PullTokenPayload is embedded in every pull token the user carries.
// The server never stores this; it re-derives it from the signature on
// each request.
type PullTokenPayload struct {
	Project   string `json:"project"`
	IssuedAt  int64  `json:"issued_at"`  // Unix seconds
	ExpiresAt int64  `json:"expires_at"` // 0 = no expiry
}

// SignedPullToken is what gets handed to the user after deploy:
//
//	<base64url(payload)>.<base64url(signature)>
//
// Both halves are needed — the server reconstructs the payload from the
// first half and verifies the second half against its private key.
type SignedPullToken struct {
	raw string
}

func (t SignedPullToken) String() string { return t.raw }

// ── Key management ────────────────────────────────────────────────────────────

// GenerateSigningKey creates a new Ed25519 keypair for a project.
// Only the private key seed is stored (in the registry). The public key is
// derived from it on every verification — no separate storage needed.
func GenerateSigningKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate Ed25519 key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), nil
}

func privateKeyFromB64(b64 string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode signing key: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid signing key length")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ── Key persistence ───────────────────────────────────────────────────────────

// SetSigningKey generates a fresh Ed25519 key for projectName and persists
// it to the registry. Called once at the end of the deploy flow — this is
// what populates p.SigningKey so that IssueToken can find it.
// Safe to call again to rotate: replaces the existing key, immediately
// invalidating any previously issued tokens.
func SetSigningKey(projectName string) error {
	key, err := GenerateSigningKey()
	if err != nil {
		return err
	}
	reg, err := loadRegistry()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	p, ok := reg[projectName]
	if !ok {
		return fmt.Errorf("project %q not found in registry", projectName)
	}
	p.SigningKey = key
	reg[projectName] = p
	return saveRegistry(reg)
}

// ── Token issuance ────────────────────────────────────────────────────────────

// IssueToken signs a new pull token for projectName using the private key
// stored in the registry. The token is returned to hand to the user — it is
// never stored server-side.
//
// Typical deploy flow:
//
//	if err := core.SetSigningKey(projectName); err != nil { ... }
//	token, err := core.IssueToken(projectName, 0)  // 0 = no expiry
//	fmt.Printf("[✓] Pull token (save this):\n%s\n", token)
func IssueToken(projectName string, ttl time.Duration) (SignedPullToken, error) {
	reg, err := loadRegistry()
	if err != nil {
		return SignedPullToken{}, fmt.Errorf("load registry: %w", err)
	}
	p, ok := reg[projectName]
	if !ok {
		return SignedPullToken{}, fmt.Errorf("project %q not found", projectName)
	}
	if p.SigningKey == "" {
		return SignedPullToken{}, fmt.Errorf("project %q has no signing key — call SetSigningKey first", projectName)
	}

	privKey, err := privateKeyFromB64(p.SigningKey)
	if err != nil {
		return SignedPullToken{}, err
	}

	payload := PullTokenPayload{
		Project:  projectName,
		IssuedAt: time.Now().Unix(),
	}
	if ttl > 0 {
		payload.ExpiresAt = time.Now().Add(ttl).Unix()
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return SignedPullToken{}, fmt.Errorf("marshal payload: %w", err)
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := ed25519.Sign(privKey, []byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return SignedPullToken{raw: payloadB64 + "." + sigB64}, nil
}

// ── Token verification ────────────────────────────────────────────────────────

// VerifyToken parses and verifies a signed pull token.
// Returns the embedded payload on success. The project name inside the
// payload is authoritative — the caller must not trust any project name
// supplied separately in the request.
func VerifyToken(raw string) (PullTokenPayload, error) {
	parts := strings.SplitN(raw, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return PullTokenPayload{}, fmt.Errorf("malformed token")
	}
	payloadB64, sigB64 := parts[0], parts[1]

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return PullTokenPayload{}, fmt.Errorf("decode payload: %w", err)
	}

	var payload PullTokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return PullTokenPayload{}, fmt.Errorf("parse payload: %w", err)
	}
	if payload.Project == "" {
		return PullTokenPayload{}, fmt.Errorf("payload missing project")
	}

	// Look up the signing key for the project named inside the payload.
	// An attacker cannot forge a different project name without the
	// corresponding private key.
	reg, err := loadRegistry()
	if err != nil {
		return PullTokenPayload{}, fmt.Errorf("load registry: %w", err)
	}
	p, ok := reg[payload.Project]
	if !ok || p.SigningKey == "" {
		return PullTokenPayload{}, fmt.Errorf("unknown project or no signing key")
	}

	privKey, err := privateKeyFromB64(p.SigningKey)
	if err != nil {
		return PullTokenPayload{}, err
	}
	pubKey := privKey.Public().(ed25519.PublicKey)

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return PullTokenPayload{}, fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(pubKey, []byte(payloadB64), sig) {
		return PullTokenPayload{}, fmt.Errorf("invalid signature")
	}

	if payload.ExpiresAt != 0 && time.Now().Unix() > payload.ExpiresAt {
		return PullTokenPayload{}, fmt.Errorf("token expired")
	}

	return payload, nil
}

// ── Key rotation ──────────────────────────────────────────────────────────────

// RotateSigningKey replaces the keypair for projectName and returns a fresh
// token signed with the new key. The old key is gone immediately — any
// previously issued tokens stop working. Delegates to SetSigningKey so the
// rotation and persistence logic live in one place.
func RotateSigningKey(projectName string) (SignedPullToken, error) {
	if err := SetSigningKey(projectName); err != nil {
		return SignedPullToken{}, err
	}
	fmt.Printf("[✓] Signing key rotated for %s — previous tokens are now invalid\n", projectName)
	return IssueToken(projectName, 0)
}
