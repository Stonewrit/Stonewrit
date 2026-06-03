package core

// Record is one event's verifiable facts, independent of any storage. The
// server fetches these from its database row; the public verifier reads them
// from an exported evidence bundle. Either way, verification is the same pure
// recomputation over these values.
type Record struct {
	// PreviousEventHash is the prior event's event_hash, or nil at the start
	// of a chain (chain position 1). When nil, verification treats the
	// predecessor as the literal "genesis".
	PreviousEventHash *string
	// PayloadHash and EventHash are the stored hashes to check against.
	PayloadHash string
	EventHash   string
	// ChainPosition is the 1-based position of this event in its chain.
	ChainPosition int64
	// Content, when set, lets VerifyEvent recompute and check PayloadHash from
	// the raw event content. When nil, only the event-hash linkage is checked
	// (the chain is still proven internally consistent, but the payload is
	// taken as given).
	Content *ContentInput
}

// Checks reports each independent verification outcome for one event.
type Checks struct {
	PayloadHashChecked bool `json:"payload_hash_checked"`
	PayloadHashMatches bool `json:"payload_hash_matches"`
	EventHashMatches   bool `json:"event_hash_matches"`
	LinksToPrevious    bool `json:"links_to_previous"`
}

// Result is the verdict for a single event.
type Result struct {
	Valid    bool              `json:"valid"`
	Checks   Checks            `json:"checks"`
	Computed map[string]string `json:"computed"`
}

// VerifyEvent recomputes the event hash (and the payload hash when Content is
// present) for one record and reports whether the stored values match.
func VerifyEvent(rec Record) Result {
	checks := Checks{}
	computed := map[string]string{}

	prev := ""
	if rec.PreviousEventHash != nil {
		prev = *rec.PreviousEventHash
	}

	recomputedEvent := EventHash(prev, rec.PayloadHash, rec.ChainPosition)
	computed["event_hash"] = recomputedEvent
	checks.EventHashMatches = recomputedEvent == rec.EventHash

	// previous_event_hash is null only at position 1 (genesis).
	if rec.ChainPosition == 1 {
		checks.LinksToPrevious = rec.PreviousEventHash == nil
	} else {
		checks.LinksToPrevious = rec.PreviousEventHash != nil && *rec.PreviousEventHash != ""
	}

	if rec.Content != nil {
		checks.PayloadHashChecked = true
		if h, err := PayloadHash(BuildContentPayload(*rec.Content)); err == nil {
			computed["payload_hash"] = h
			checks.PayloadHashMatches = h == rec.PayloadHash
		}
	}

	valid := checks.EventHashMatches && checks.LinksToPrevious &&
		(!checks.PayloadHashChecked || checks.PayloadHashMatches)

	return Result{Valid: valid, Checks: checks, Computed: computed}
}

// ChainBreak describes the first event at which a chain fails verification.
type ChainBreak struct {
	Position int64  `json:"position"`
	Reason   string `json:"reason"`
	Expected string `json:"expected,omitempty"`
	Stored   string `json:"stored,omitempty"`
}

// ChainResult is the verdict for a whole chain walk.
type ChainResult struct {
	Valid          bool        `json:"valid"`
	EventsVerified int         `json:"events_verified"`
	Count          int         `json:"count"`
	Break          *ChainBreak `json:"break,omitempty"`
}

// VerifyChain walks records in ascending chain order, checking that each event
// links to the running tip and that every event hash (and payload hash, when
// content is present) recomputes. It stops at the first break.
//
// Records must be sorted by ChainPosition. An explicit PreviousEventHash, when
// present, is checked against the running tip; when absent, the running tip is
// used directly so a bundle that omits the denormalized previous hash still
// verifies.
func VerifyChain(records []Record) ChainResult {
	res := ChainResult{Count: len(records)}
	prev := ""
	for _, rec := range records {
		if rec.PreviousEventHash != nil && *rec.PreviousEventHash != prev {
			res.Break = &ChainBreak{
				Position: rec.ChainPosition,
				Reason:   "previous_event_hash does not match running tip",
				Expected: prev,
				Stored:   *rec.PreviousEventHash,
			}
			return res
		}

		recomputedEvent := EventHash(prev, rec.PayloadHash, rec.ChainPosition)
		if recomputedEvent != rec.EventHash {
			res.Break = &ChainBreak{
				Position: rec.ChainPosition,
				Reason:   "event_hash mismatch",
				Expected: recomputedEvent,
				Stored:   rec.EventHash,
			}
			return res
		}

		if rec.Content != nil {
			h, err := PayloadHash(BuildContentPayload(*rec.Content))
			if err != nil || h != rec.PayloadHash {
				res.Break = &ChainBreak{
					Position: rec.ChainPosition,
					Reason:   "payload_hash mismatch",
					Expected: h,
					Stored:   rec.PayloadHash,
				}
				return res
			}
		}

		prev = rec.EventHash
		res.EventsVerified++
	}

	res.Valid = res.Break == nil
	return res
}
