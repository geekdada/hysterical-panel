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

const alphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Alphanumeric returns a cryptographically random string of n characters drawn
// uniformly from [A-Za-z0-9]. Unlike New it yields a clean alphabet with no
// '-' or '_', which keeps Hysteria auth keys easy to copy and embed.
func Alphanumeric(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("token: n must be > 0")
	}
	out := make([]byte, n)
	buf := make([]byte, n)
	filled := 0
	for filled < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("token: read random: %w", err)
		}
		for _, b := range buf {
			// 256 % 62 == 8, so values 248..255 would bias the modulo;
			// reject them and keep the uniform 0..247 range (4*62).
			if b >= 248 {
				continue
			}
			out[filled] = alphanumeric[b%62]
			filled++
			if filled == n {
				break
			}
		}
	}
	return string(out), nil
}
