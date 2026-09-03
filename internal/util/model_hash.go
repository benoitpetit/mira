package util

import (
	"crypto/sha256"
	"encoding/hex"
)

// ComputeModelHash computes a consistent model hash using SHA-256.
// Returns the first 16 hex characters (8 bytes) of the SHA-256 hash of the model name.
// This is the canonical hash function used by both MIRA and SOUL.
func ComputeModelHash(modelName string) string {
	hash := sha256.Sum256([]byte(modelName))
	return hex.EncodeToString(hash[:8])
}
