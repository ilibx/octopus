// Package hkdfcompat provides HKDF implementation compatible with Go 1.19.
// This is a backport of crypto/hkdf from Go 1.21+ for compatibility.
package hkdfcompat

import (
	"crypto/sha256"
	"errors"
	"hash"
	"io"

	"golang.org/x/crypto/hkdf"
)

// Key derives a key using HKDF-SHA256.
// This function matches the signature of crypto/hkdf.Key from Go 1.21+.
//
// Parameters:
//   - h: hash function constructor (typically sha256.New)
//   - ikm: input keying material
//   - salt: optional salt value (can be nil)
//   - info: optional context and application specific information (can be nil)
//   - length: desired key length in bytes
//
// Returns the derived key or an error if the length is too large.
func Key(h func() hash.Hash, ikm, salt, info []byte, length int) ([]byte, error) {
	hkdfReader := hkdf.New(h, ikm, salt, info)
	key := make([]byte, length)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, errors.New("hkdf: invalid key length")
	}
	return key, nil
}

// Expand is a wrapper around hkdf.Expand for compatibility.
func Expand(h func() hash.Hash, prk, info []byte, length int) ([]byte, error) {
	hkdfReader := hkdf.Expand(h, prk, info)
	key := make([]byte, length)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, errors.New("hkdf: invalid key length")
	}
	return key, nil
}

// Extract is a wrapper around hkdf.Extract for compatibility.
func Extract(h func() hash.Hash, salt, ikm []byte) []byte {
	return hkdf.Extract(h, salt, ikm)
}

// SHA256Key derives a 32-byte key using HKDF-SHA256.
// Convenience function for common use case.
func SHA256Key(ikm, salt, info []byte, length int) ([]byte, error) {
	return Key(sha256.New, ikm, salt, info, length)
}
