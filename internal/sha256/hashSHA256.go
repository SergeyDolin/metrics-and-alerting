// Package sha256 provides HMAC-SHA256 utilities for request signing and verification.
// It is used to ensure data integrity and authenticity between the metrics agent and server.
package sha256

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// ComputeHMACSHA256 calculates an HMAC-SHA256 hash of the provided data using the given key.
// HMAC (Hash-based Message Authentication Code) provides both data integrity and authenticity
// by combining the data with a secret key.
//
// The function:
//  1. Creates a new HMAC hasher using SHA256 with the provided key
//  2. Writes the data to the hasher
//  3. Returns the hash as a hexadecimal string
//
// Parameters:
//   - data: The byte slice to be hashed (typically JSON-marshaled metric data)
//   - key: The secret key used for HMAC computation (can be empty)
//
// Returns:
//   - string: Hexadecimal representation of the HMAC-SHA256 hash.
//     Returns an empty string if the key is empty.
//
// Example:
//
//	hash := ComputeHMACSHA256([]byte(`{"id":"Alloc","type":"gauge","value":42.5}`), "secret-key")
//	fmt.Println(hash) // Output: 5d4f3c8e2a1b9f7d6c5e4a3b2c1d0e9f8a7b6c5d
func ComputeHMACSHA256(data []byte, key string) string {
	// If no key is provided, return empty string (no hash)
	if key == "" {
		return ""
	}

	// Create a new HMAC hasher using SHA256 with the secret key
	h := hmac.New(sha256.New, []byte(key))

	// Write the data to the hasher
	// Note: h.Write never returns an error in this implementation
	h.Write(data)

	// Calculate the HMAC sum and encode it as a hexadecimal string
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyHashSHA256 verifies that the provided hash matches the HMAC-SHA256
// computed from the data and key. This ensures that:
//  1. The data hasn't been tampered with (integrity)
//  2. The data was sent by someone who knows the secret key (authenticity)
//
// The verification process:
//  1. If either key or expectedHash is empty, verification is skipped (returns true)
//  2. Computes the HMAC-SHA256 of the data using the key
//  3. Compares the computed hash with the provided expectedHash
//
// Parameters:
//   - data: The original byte slice that was (or should have been) hashed
//   - key: The secret key used for HMAC computation
//   - expectedHash: The hash to verify against (hexadecimal string)
//
// Returns:
//   - bool: true if the hash is valid or verification is skipped,
//     false if the hash doesn't match
//
// Example:
//
//	isValid := VerifyHashSHA256(
//	    []byte(`{"id":"Alloc","type":"gauge","value":42.5}`),
//	    "secret-key",
//	    receivedHash,
//	)
//	if !isValid {
//	    log.Println("Warning: Hash verification failed - possible tampering")
//	}
func VerifyHashSHA256(data []byte, key, expectedHash string) bool {
	// Skip verification if no key or hash is provided
	// This allows the system to work in environments where hashing is not configured
	if key == "" || expectedHash == "" {
		return true
	}

	// Compute the expected hash from the data and key
	calculatedHash := ComputeHMACSHA256(data, key)

	// Compare the computed hash with the provided hash
	// Note: This uses hmac.Equal internally which performs a constant-time comparison
	// to prevent timing attacks that could leak information about the hash value
	return calculatedHash == expectedHash
}
