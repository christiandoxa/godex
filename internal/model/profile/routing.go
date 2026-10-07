package profile

import (
	"crypto/sha256"
	"encoding/hex"
)

// RoutingID is the stable, non-secret runtime identity for a profile routing seed.
func RoutingID(seed string) string {
	digest := sha256.Sum256([]byte("profile:" + seed))
	return hex.EncodeToString(digest[:16])
}
