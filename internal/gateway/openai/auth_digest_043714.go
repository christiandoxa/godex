package openai

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Auth digests stay in private process memory and never include a bearer
// token, refresh token, or account ID in user-facing output or persisted DTOs.
func openAIAuthDigest043714(auth proxymodel.Auth) ([32]byte, bool) {
	if auth.AccessToken == "" {
		return [32]byte{}, false
	}
	digest := sha256.New()
	writeAuthField043714(digest, auth.AccessToken)
	writeAuthField043714(digest, auth.AccountID)
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result, true
}

func writeAuthField043714(digest hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write([]byte(value))
}
