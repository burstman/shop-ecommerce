package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// OrderTrackingToken builds a stable, secret-signed token for the public order
// tracking page. The token is derived only from the order ID and SUPERKIT_SECRET,
// so no column is needed: /tracking?id={id}&t={token} is self-validating.
func OrderTrackingToken(orderID uint) string {
	secret := os.Getenv("SUPERKIT_SECRET")
	if secret == "" {
		secret = "fallback-secret-key-change-in-production"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("order-tracking:%d", orderID)))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// ValidOrderTrackingToken reports whether token is the correct signed token for
// orderID. It does NOT compare with constant time to avoid leaking any info in
// the unlikely event the URL is fragmented; the token itself is unguessable.
func ValidOrderTrackingToken(orderID uint, token string) bool {
	return token != "" && hmac.Equal([]byte(OrderTrackingToken(orderID)), []byte(token))
}