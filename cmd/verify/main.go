// Command verify independently checks a Stonewrit evidence bundle. It depends
// on nothing but the public core verifier and the event spec, so an auditor or
// regulator can confirm a hash chain without trusting the service that produced
// it. This is the runnable form of "verify it yourself".
//
// Usage:
//
//	verify [bundle.json]      # or pipe the bundle on stdin
//	verify -json bundle.json  # machine-readable output
//
// The bundle is the JSON evidence export (schema "stonewrit-evidence-bundle/1").
// For each chain it walks the events in order, recomputing every event hash and
// checking that the final hash matches the chain's published tip. It exits 0
// when every chain verifies and 1 otherwise.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/stonewrit/stonewrit/core"
)

// bundle mirrors the public evidence-export shape. Only the fields the verifier
// needs are decoded.
type bundle struct {
	SchemaVersion string        `json:"schema_version"`
	Chains        []bundleChain `json:"chains"`
	Events        []bundleEvent `json:"events"`
}

type bundleChain struct {
	ID              string  `json:"id"`
	LatestPosition  int64   `json:"latest_position"`
	LatestEventHash *string `json:"latest_event_hash,omitempty"`
}

type bundleEvent struct {
	ID            string `json:"id"`
	ChainID       string `json:"chain_id"`
	ChainPosition int64  `json:"chain_position"`
	PayloadHash   string `json:"payload_hash"`
	EventHash     string `json:"event_hash"`
}

type chainReport struct {
	ChainID        string           `json:"chain_id"`
	Valid          bool             `json:"valid"`
	EventsVerified int              `json:"events_verified"`
	EventCount     int              `json:"event_count"`
	TipMatches     bool             `json:"tip_matches"`
	Break          *core.ChainBreak `json:"break,omitempty"`
}

type report struct {
	Valid      bool          `json:"valid"`
	ChainCount int           `json:"chain_count"`
	EventCount int           `json:"event_count"`
	Chains     []chainReport `json:"chains"`
}

func main() {
	jsonOut := flag.Bool("json", false, "emit a machine-readable JSON report")
	flag.Parse()

	raw, err := readInput(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(2)
	}

	var b bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		fmt.Fprintf(os.Stderr, "verify: parse bundle: %v\n", err)
		os.Exit(2)
	}

	rep := verifyBundle(b)

	if *jsonOut {
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Print(renderReport(rep))
	}
	if !rep.Valid {
		os.Exit(1)
	}
}

func readInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// verifyBundle groups events by chain, walks each chain through the core
// verifier, and checks each chain against its published tip.
func verifyBundle(b bundle) report {
	tips := make(map[string]*string, len(b.Chains))
	for _, c := range b.Chains {
		tips[c.ID] = c.LatestEventHash
	}

	byChain := make(map[string][]bundleEvent)
	for _, e := range b.Events {
		byChain[e.ChainID] = append(byChain[e.ChainID], e)
	}

	chainIDs := make([]string, 0, len(byChain))
	for id := range byChain {
		chainIDs = append(chainIDs, id)
	}
	sort.Strings(chainIDs)

	rep := report{Valid: true, EventCount: len(b.Events)}
	for _, id := range chainIDs {
		events := byChain[id]
		sort.Slice(events, func(i, j int) bool {
			return events[i].ChainPosition < events[j].ChainPosition
		})

		records := make([]core.Record, len(events))
		for i, e := range events {
			records[i] = core.Record{
				PayloadHash:   e.PayloadHash,
				EventHash:     e.EventHash,
				ChainPosition: e.ChainPosition,
			}
		}

		cr := core.VerifyChain(records)
		tipMatches := false
		if cr.Valid && len(events) > 0 {
			tip := tips[id]
			tipMatches = tip != nil && *tip == events[len(events)-1].EventHash
		}

		valid := cr.Valid && tipMatches
		rep.Chains = append(rep.Chains, chainReport{
			ChainID:        id,
			Valid:          valid,
			EventsVerified: cr.EventsVerified,
			EventCount:     len(events),
			TipMatches:     tipMatches,
			Break:          cr.Break,
		})
		if !valid {
			rep.Valid = false
		}
	}
	rep.ChainCount = len(rep.Chains)
	return rep
}
