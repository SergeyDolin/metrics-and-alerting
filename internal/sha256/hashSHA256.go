package sha256

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func ComputeHMACSHA256(data []byte, key string) string {
	if key == "" {
		return ""
	}

	h := hmac.New(sha256.New, []byte(key))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func VerifyHashSHA256(data []byte, key, expectedHash string) bool {
	if key == "" || expectedHash == "" {
		return true
	}

	calculatedHash := ComputeHMACSHA256(data, key)
	return calculatedHash == expectedHash
}
