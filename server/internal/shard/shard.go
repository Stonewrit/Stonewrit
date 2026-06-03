// Package shard owns the per-environment fan-out decision.
//
// 8 shards per environment by default. Events hash-distribute by
// `actor.id` so a single actor's events stay in order within their
// own shard, while different actors spread load. Events without an
// actor.id fall back to random selection.
package shard

import (
	"hash/fnv"
	"math/rand/v2"
)

const DefaultCount = 8

// IndexFor returns 0..count-1 deterministically from key; random if empty.
func IndexFor(key string, count int) int {
	if count <= 0 {
		count = DefaultCount
	}
	if key == "" {
		return rand.IntN(count)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(count))
}
