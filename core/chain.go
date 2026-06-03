package core

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Frozen identifiers for the hash scheme. A change to canonicalization or the
// pre-image is a new CanonicalizationVersion (for example "jcs-v2"), never a
// silent edit of these values.
const (
	// HashPrefix is the textual algorithm tag, stored verbatim in the
	// payload_hash, event_hash, and latest_event_hash columns.
	HashPrefix = "sha256:"
	// CanonicalizationVersion names the canonicalization rules in force.
	CanonicalizationVersion = "jcs-v1"
	// HashAlgorithm names the digest in force.
	HashAlgorithm = "sha256"
)

// PayloadHash returns the canonical content hash of a payload:
//
//	HashPrefix + sha256hex( CanonicalJSON(payload) )
func PayloadHash(payload any) (string, error) {
	canon, err := CanonicalJSON(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return HashPrefix + hex.EncodeToString(sum[:]), nil
}

// EventHash chains a payload hash to its predecessor and position:
//
//	HashPrefix + sha256hex( previousHash + ":" + payloadHash + ":" + position )
//
// previousHash is the prior event's event_hash, or the empty string at the
// start of a chain, which is normalized to the literal "genesis". position is a
// base-10 decimal with no padding. The pre-image chains the payload_hash string
// (already prefixed with "sha256:"), not the raw canonical bytes, because the
// payload hash already commits to the content.
func EventHash(previousHash, payloadHash string, position int64) string {
	prev := previousHash
	if prev == "" {
		prev = "genesis"
	}
	input := prev + ":" + payloadHash + ":" + strconv.FormatInt(position, 10)
	sum := sha256.Sum256([]byte(input))
	return HashPrefix + hex.EncodeToString(sum[:])
}
