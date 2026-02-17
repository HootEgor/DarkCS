// Package fileurl provides HMAC-signed URL generation and verification for file downloads.
// URLs expire after a configurable TTL, preventing token leakage via browser history,
// server logs, or Referer headers.
package fileurl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// SignURL returns a relative URL path with HMAC signature and expiry query parameters.
// The signature covers "{fileID}:{expiresUnix}" using HMAC-SHA256.
func SignURL(fileID, secret string, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	sig := computeHMAC(fileID, expires, secret)
	return fmt.Sprintf("/crm/files/%s?expires=%d&sig=%s", fileID, expires, sig)
}

// Verify checks that the HMAC signature is valid and the URL has not expired.
func Verify(fileID, expires, sig, secret string) bool {
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	expected := computeHMAC(fileID, exp, secret)
	return hmac.Equal([]byte(sig), []byte(expected))
}

func computeHMAC(fileID string, expires int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%s:%d", fileID, expires)))
	return hex.EncodeToString(mac.Sum(nil))
}
