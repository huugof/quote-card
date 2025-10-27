package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func HashStrings(parts ...string) string {
	return HashJSON(parts)
}

func HashJSON(value any) string {
	data, _ := json.Marshal(value)
	return HashBytes(data)
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
