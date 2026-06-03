// Package core holds the pure, deterministic hash-chain primitives that define
// Stonewrit's tamper-evidence: canonicalization, payload hashing, event-hash
// chain linkage, and verification.
//
// Everything here is pure. The same input bytes produce the same output hash on
// any machine, in any language, forever. There are no database, HTTP, clock,
// configuration, or logging dependencies. The ingest server and the public
// verifier import this exact package, which is what makes "verify it yourself"
// a true statement rather than a promise.
//
// The canonicalization and pre-image scheme are frozen. Any change to them is a
// new CanonicalizationVersion, never a silent edit, because a change would
// invalidate the event hashes of every chain ever produced.
package core

import (
	"encoding/json"
	"fmt"

	"github.com/gowebpki/jcs"
)

// CanonicalJSON returns the RFC 8785 (JCS) canonical UTF-8 encoding of v.
//
// Do not rely on a language's default JSON serializer to be JCS compliant. Key
// ordering, number formatting, and UTF-8 handling must be byte-identical across
// every implementation, which is what the frozen conformance vectors in
// testdata/golden_vectors.json pin down.
func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	canon, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("jcs transform: %w", err)
	}
	return canon, nil
}
