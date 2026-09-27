package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	base62Charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// UserIDPrefix prefixes encoded user display ids.
	UserIDPrefix = "USR_"
	// OrgIDPrefix prefixes encoded organization display ids.
	OrgIDPrefix = "ORG_"
)

// EncodeBase62 encodes a positive int64 to a base62 string using charset
// 0-9A-Za-z.
func EncodeBase62(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base62Charset[n%62]
		n /= 62
	}
	return string(buf[i:])
}

// DecodeBase62 decodes a base62 string back to int64.
func DecodeBase62(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty base62 string")
	}
	var result int64
	for _, c := range s {
		idx := strings.IndexRune(base62Charset, c)
		if idx < 0 {
			return 0, fmt.Errorf("invalid base62 character: %c", c)
		}
		result = result*62 + int64(idx)
	}
	return result, nil
}

// deriveDisplayIDKey derives a 63-bit mask from the HMAC of the given secret
// key.  The top bit is cleared so the masked value always stays a positive
// int64.
func deriveDisplayIDKey(secretKey string) int64 {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte("url-shortner:display-id"))
	sum := binary.BigEndian.Uint64(mac.Sum(nil)[:8])
	return int64(sum & 0x7FFFFFFFFFFFFFFF)
}

// EncodeID encodes a raw int64 identifier into an obfuscated display id of the
// form "{prefix}{base62}".  The id is XOR-masked with a key derived from the
// given secret key, so only holders of the same secret key can decode it back.
//
// Example:
//
//	displayID := utils.EncodeID(123456789, utils.OrgIDPrefix, "my-secret-key")
//	// displayID == "ORG_8M0kX..."
func EncodeID(id int64, prefix, secretKey string) string {
	return prefix + EncodeBase62(id^deriveDisplayIDKey(secretKey))
}

// DecodeID reverses EncodeID: it skips the given prefix, base62-decodes the
// payload, and XOR-unmasks using the key derived from the secret key.
func DecodeID(encoded, prefix, secretKey string) (int64, error) {
	payload := strings.TrimPrefix(encoded, prefix)
	if payload == encoded {
		return 0, fmt.Errorf("missing %q prefix", prefix)
	}

	masked, err := DecodeBase62(payload)
	if err != nil {
		return 0, fmt.Errorf("invalid base62 payload: %w", err)
	}

	return masked ^ deriveDisplayIDKey(secretKey), nil
}
