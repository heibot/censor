package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
)

// HashText returns a SHA256 hash of the text content.
func HashText(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

// HashURL returns a hash of the URL for deduplication.
func HashURL(url string) string {
	h := sha256.New()
	h.Write([]byte(url))
	return hex.EncodeToString(h.Sum(nil))
}

// QuickHash returns a fast FNV-1a hash for internal use.
func QuickHash(data string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(data))
	return h.Sum64()
}

// TruncateHash returns a truncated hash for display purposes.
func TruncateHash(hash string, length int) string {
	if len(hash) <= length {
		return hash
	}
	return hash[:length]
}
