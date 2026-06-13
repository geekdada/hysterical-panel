// Package token generates URL-safe random tokens for invite codes and
// system-generated auth strings.
package token

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// New returns a cryptographically random, URL-safe token derived from nBytes
// of entropy. The result is base64 raw-url encoded (no padding), so its length
// is ceil(nBytes*4/3) characters.
func New(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", fmt.Errorf("token: nBytes must be > 0")
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
