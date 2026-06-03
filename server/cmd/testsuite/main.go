// Stonewrit end-to-end test runner. Go port of the original
// apps/api/scripts/test-suite.ts. Sections: sanity → variety → validation →
// boundary → adversarial → idempotency → race → composite → chain → agents →
// burst. Same accept-then-seal semantics (POST returns 202; verify polls until
// state=sealed).
//
// Run with:
//
//	STONEWRIT_API_KEY=stonewrit_live_xxx go run ./cmd/testsuite
//
// Override target: STONEWRIT_API_URL=http://localhost:3002 (default).
// Skip sections: --skip burst,race
// Run only specific sections: --only sanity,variety
//
// The `agents` section asserts the observe-only agent scope_check. Because the
// registry is dashboard-managed (this runner only holds an API key), it needs a
// pre-registered agent and is skipped unless both are set:
//
//	STONEWRIT_TEST_AGENT_ID=<agent's Agent ID>      # = actor.id it reports
//	STONEWRIT_TEST_AGENT_SCOPE=<one authorized scope> # = an action.category
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stonewrit/stonewrit/server/internal/testkit"
)

const requestTimeout = 30 * time.Second

type config struct {
	apiURL  string
	apiKey  string
	skip    map[string]bool
	only    map[string]bool
	quiet   bool
	noColor bool
}

type results struct {
	pass      atomic.Int64
	fail      atomic.Int64
	failures  []string
	mu        sync.Mutex
	startedAt time.Time
}

func (r *results) addFailure(s string) {
	r.fail.Add(1)
	r.mu.Lock()
	r.failures = append(r.failures, s)
	r.mu.Unlock()
}

type eventResponse struct {
	Status     int
	Body       responseBody
	RetryAfter int // seconds, parsed from the Retry-After header on 429
}

type responseBody struct {
	ID            string          `json:"id,omitempty"`
	Status        string          `json:"status,omitempty"`
	PayloadHash   string          `json:"payload_hash,omitempty"`
	AcceptedAt    string          `json:"accepted_at,omitempty"`
	EnvironmentID string          `json:"environment_id,omitempty"`
	State         string          `json:"state,omitempty"`
	Valid         bool            `json:"valid,omitempty"`
	Checks        json.RawMessage `json:"checks,omitempty"`
	Error         *errorBody      `json:"error,omitempty"`
}

type errorBody struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

var sectionOrder = []string{
	"sanity", "variety", "validation", "boundary", "adversarial",
	"idempotency", "race", "burst", "composite", "chain", "agents",
}

func main() {
	cfg := parseFlags()
	if cfg.apiKey == "" {
		fmt.Fprintln(os.Stderr,
			"Missing STONEWRIT_API_KEY. Create an API key in the dashboard "+
				"(Projects → API Keys → Create), then:\n"+
				"  export STONEWRIT_API_KEY=stonewrit_live_xxxxxxxxxxxxxxxx")
		os.Exit(1)
	}

	r := &results{startedAt: time.Now()}

	runSanity(cfg, r)
	runVariety(cfg, r)
	runValidation(cfg, r)
	runBoundary(cfg, r)
	runAdversarial(cfg, r)
	runIdempotency(cfg, r)
	runRace(cfg, r)
	runComposite(cfg, r) // moved before burst so it gets a fresh budget window
	runChain(cfg, r)
	runAgents(cfg, r)
	runBurst(cfg, r)

	printSummary(cfg, r)
	if r.fail.Load() > 0 {
		os.Exit(1)
	}
}

func parseFlags() *config {
	skipFlag := flag.String("skip", "", "comma-list of sections to skip")
	onlyFlag := flag.String("only", "", "comma-list of sections to run exclusively")
	noColorFlag := flag.Bool("no-color", false, "plain ASCII output")
	quietFlag := flag.Bool("quiet", false, "only print section headers + summary")
	flag.Parse()

	apiURL := os.Getenv("STONEWRIT_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:3002"
	}
	apiURL = strings.TrimRight(apiURL, "/")

	cfg := &config{
		apiURL:  apiURL,
		apiKey:  os.Getenv("STONEWRIT_API_KEY"),
		quiet:   *quietFlag,
		noColor: *noColorFlag,
		skip:    map[string]bool{},
		only:    map[string]bool{},
	}
	for _, s := range strings.Split(*skipFlag, ",") {
		if s = strings.TrimSpace(s); s != "" {
			cfg.skip[s] = true
		}
	}
	for _, s := range strings.Split(*onlyFlag, ",") {
		if s = strings.TrimSpace(s); s != "" {
			cfg.only[s] = true
		}
	}
	return cfg
}

func shouldRun(cfg *config, name string) bool {
	if len(cfg.only) > 0 {
		return cfg.only[name]
	}
	return !cfg.skip[name]
}

func section(cfg *config, name string, body func()) {
	if !shouldRun(cfg, name) {
		log(cfg, gray(cfg, fmt.Sprintf("\n╌╌ %s (skipped) ╌╌", name)))
		return
	}
	log(cfg, bold(cfg, fmt.Sprintf("\n── %s ──", name)))
	body()
}

func check(cfg *config, r *results, label string, fn func() string) {
	failure := fn()
	if failure == "" {
		r.pass.Add(1)
		log(cfg, fmt.Sprintf("  %s %s", green(cfg, "✓"), label))
		return
	}
	r.addFailure(fmt.Sprintf("%s - %s", label, failure))
	log(cfg, fmt.Sprintf("  %s %s %s", red(cfg, "✗"), label, gray(cfg, "- "+failure)))
}

/* ─── sections ────────────────────────────────────────────────────────── */

func runSanity(cfg *config, r *results) {
	section(cfg, "sanity", func() {
		check(cfg, r, "health endpoint reachable", func() string {
			res, err := http.Get(cfg.apiURL + "/health")
			if err != nil {
				return fmt.Sprintf("health unreachable: %v", err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Sprintf("health returned %d", res.StatusCode)
			}
			return ""
		})
		check(cfg, r, "ready endpoint reachable", func() string {
			res, err := http.Get(cfg.apiURL + "/ready")
			if err != nil {
				return fmt.Sprintf("ready unreachable: %v", err)
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Sprintf("ready returned %d", res.StatusCode)
			}
			return ""
		})
		check(cfg, r, "happy POST returns 202 accepted with payload_hash receipt", func() string {
			res := postEvent(cfg, testkit.GenerateEvent(testkit.GenerateOpts{}), "")
			if res.Status != 202 {
				return fmt.Sprintf("expected 202, got %d", res.Status)
			}
			if res.Body.ID == "" {
				return "response has no id"
			}
			if res.Body.PayloadHash == "" {
				return "response has no payload_hash"
			}
			if res.Body.Status != "pending" {
				return fmt.Sprintf("expected status=pending, got %q", res.Body.Status)
			}
			return ""
		})
		check(cfg, r, "verify endpoint returns valid:true once sealer catches up", func() string {
			post := postEvent(cfg, testkit.GenerateEvent(testkit.GenerateOpts{}), "")
			if post.Status != 202 || post.Body.ID == "" {
				return fmt.Sprintf("POST failed: %d", post.Status)
			}
			ver := waitForSeal(cfg, post.Body.ID, 15*time.Second)
			if !ver.Body.Valid {
				return fmt.Sprintf("verify returned valid:false: %s", string(ver.Body.Checks))
			}
			return ""
		})
	})
}

func runVariety(cfg *config, r *results) {
	section(cfg, "variety", func() {
		for _, cat := range testkit.Categories {
			cat := cat
			check(cfg, r, fmt.Sprintf("category: %s accepted", cat), func() string {
				res := postEvent(cfg, testkit.GenerateEvent(testkit.GenerateOpts{Category: cat}), "")
				if res.Status != 202 {
					return fmt.Sprintf("expected 202, got %d", res.Status)
				}
				return ""
			})
		}
	})
}

func runValidation(cfg *config, r *results) {
	section(cfg, "validation", func() {
		for _, name := range sortedKeys(testkit.InvalidScenarios) {
			name := name
			builder := testkit.InvalidScenarios[name]
			check(cfg, r, fmt.Sprintf("invalid: %s → 422", name), func() string {
				res := postEvent(cfg, builder(), "")
				if res.Status != 422 {
					return fmt.Sprintf("expected 422, got %d", res.Status)
				}
				if res.Body.Error == nil || res.Body.Error.Code != "validation_failed" {
					return fmt.Sprintf("expected error.code=validation_failed, got %+v", res.Body.Error)
				}
				return ""
			})
		}
	})
}

func runBoundary(cfg *config, r *results) {
	section(cfg, "boundary", func() {
		for _, name := range sortedKeys(testkit.BoundaryScenarios) {
			name := name
			builder := testkit.BoundaryScenarios[name]
			check(cfg, r, fmt.Sprintf("boundary: %s → 202", name), func() string {
				res := postEvent(cfg, builder(), "")
				if res.Status != 202 {
					return fmt.Sprintf("expected 202, got %d", res.Status)
				}
				return ""
			})
		}
	})
}

func runAdversarial(cfg *config, r *results) {
	section(cfg, "adversarial", func() {
		for _, name := range sortedKeys(testkit.AdversarialScenarios) {
			name := name
			builder := testkit.AdversarialScenarios[name]
			check(cfg, r, fmt.Sprintf("adversarial: %s stored safely", name), func() string {
				res := postEvent(cfg, builder(), "")
				if res.Status >= 500 {
					return fmt.Sprintf("5xx is a bug - payload broke something. status=%d", res.Status)
				}
				if res.Status != 202 && res.Status != 422 {
					return fmt.Sprintf("unexpected status %d", res.Status)
				}
				return ""
			})
		}
	})
}

func runIdempotency(cfg *config, r *results) {
	section(cfg, "idempotency", func() {
		check(cfg, r, "same key twice returns same id (cached 202 replay)", func() string {
			key := fmt.Sprintf("test-idem-%d", time.Now().UnixMilli())
			payload := testkit.GenerateEvent(testkit.GenerateOpts{})
			a := postEvent(cfg, payload, key)
			if a.Status != 202 {
				return fmt.Sprintf("first POST: expected 202, got %d", a.Status)
			}
			b := postEvent(cfg, payload, key)
			if b.Status != 202 {
				return fmt.Sprintf("second POST: expected 202 (cached), got %d", b.Status)
			}
			if a.Body.ID != b.Body.ID {
				return fmt.Sprintf("ids differ: first=%s second=%s", a.Body.ID, b.Body.ID)
			}
			return ""
		})
	})
}

func runRace(cfg *config, r *results) {
	section(cfg, "race", func() {
		check(cfg, r, "25 parallel POSTs with same idempotency key → 1 distinct id", func() string {
			key := fmt.Sprintf("test-race-%d", time.Now().UnixMilli())
			payload := testkit.GenerateEvent(testkit.GenerateOpts{})

			const n = 25
			ids := make([]string, n)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					res := postEvent(cfg, payload, key)
					ids[idx] = res.Body.ID
				}(i)
			}
			wg.Wait()

			distinct := map[string]struct{}{}
			for _, id := range ids {
				if id != "" {
					distinct[id] = struct{}{}
				}
			}
			if len(distinct) == 0 {
				return "no successful POSTs"
			}
			if len(distinct) > 1 {
				sample := []string{}
				for id := range distinct {
					sample = append(sample, id)
					if len(sample) == 3 {
						break
					}
				}
				return fmt.Sprintf("dedupe failed - %d distinct ids returned: %s…",
					len(distinct), strings.Join(sample, ", "))
			}
			return ""
		})
	})
}

func runBurst(cfg *config, r *results) {
	section(cfg, "burst", func() {
		check(cfg, r, "120 parallel POSTs do not 5xx", func() string {
			const n = 120
			statuses := make([]int, n)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					res := postEvent(cfg, testkit.GenerateEvent(testkit.GenerateOpts{}), "")
					statuses[idx] = res.Status
				}(i)
			}
			wg.Wait()

			counts := map[int]int{}
			for _, s := range statuses {
				counts[s]++
			}
			fiveXX := []string{}
			for code, count := range counts {
				if code >= 500 {
					fiveXX = append(fiveXX, fmt.Sprintf("%d:%d", code, count))
				}
			}
			if len(fiveXX) > 0 {
				return "got 5xx: " + strings.Join(fiveXX, ", ")
			}
			parts := []string{}
			for _, code := range sortedIntKeys(counts) {
				parts = append(parts, fmt.Sprintf("%d:%d", code, counts[code]))
			}
			log(cfg, "      ↳ status spread: "+strings.Join(parts, ", "))
			return ""
		})
	})
}

func runComposite(cfg *config, r *results) {
	section(cfg, "composite", func() {
		for _, name := range sortedKeys(testkit.CompositeScenarios) {
			name := name
			builder := testkit.CompositeScenarios[name]
			check(cfg, r, fmt.Sprintf("composite: %s every event 202", name), func() string {
				events := builder()
				for _, e := range events {
					res := postEventTolerant(cfg, e)
					if res.Status != 202 {
						eventType, _ := e["event_type"].(string)
						return fmt.Sprintf("event %q returned %d %s",
							eventType, res.Status, errBrief(res.Body.Error))
					}
				}
				return ""
			})
		}
	})
}

func runChain(cfg *config, r *results) {
	section(cfg, "chain", func() {
		check(cfg, r, "POST 10 events and verify each → all valid:true", func() string {
			ids := []string{}
			for i := 0; i < 10; i++ {
				res := postEventTolerant(cfg, testkit.GenerateEvent(testkit.GenerateOpts{}))
				if res.Status != 202 || res.Body.ID == "" {
					return fmt.Sprintf("POST %d failed: %d", i, res.Status)
				}
				ids = append(ids, res.Body.ID)
			}
			for _, id := range ids {
				v := waitForSeal(cfg, id, 15*time.Second)
				if !v.Body.Valid {
					return fmt.Sprintf("event %s valid:false → %s", id, string(v.Body.Checks))
				}
			}
			return ""
		})
	})
}

// runAgents exercises the observe-only agent scope_check. It needs a
// pre-registered agent because the registry is dashboard-managed (the suite
// only holds an API key, it cannot create one). Configure it with:
//
//	STONEWRIT_TEST_AGENT_ID    the agent's Agent ID (= actor.id it reports)
//	STONEWRIT_TEST_AGENT_SCOPE one action.category the agent IS authorized for
//
// The section is skipped (not failed) when those are unset.
func runAgents(cfg *config, r *results) {
	section(cfg, "agents", func() {
		agentID := strings.TrimSpace(os.Getenv("STONEWRIT_TEST_AGENT_ID"))
		inScope := strings.TrimSpace(os.Getenv("STONEWRIT_TEST_AGENT_SCOPE"))
		if agentID == "" || inScope == "" {
			log(cfg, gray(cfg, "  ↳ skipped: set STONEWRIT_TEST_AGENT_ID + "+
				"STONEWRIT_TEST_AGENT_SCOPE to a registered agent's Agent ID and "+
				"one of its authorized scopes (action.category). Register the agent "+
				"in the dashboard (Project → Agents) first."))
			return
		}

		const deniedCat = "testsuite.denied.scope"
		const unregisteredID = "testsuite-unregistered-agent-zzz"

		// sealAndScopeCheck posts an agent event, waits for the sealer, and returns
		// the scope_check object off the sealed event (nil if absent).
		sealAndScopeCheck := func(payload map[string]any) (map[string]any, string) {
			post := postEventTolerant(cfg, payload)
			if post.Status != 202 || post.Body.ID == "" {
				return nil, fmt.Sprintf("POST failed: %d", post.Status)
			}
			ver := waitForSeal(cfg, post.Body.ID, 15*time.Second)
			if ver.Body.State != "sealed" {
				return nil, "event did not seal within 15s"
			}
			return fetchScopeCheck(cfg, post.Body.ID)
		}

		check(cfg, r, "in-scope agent action → scope_check in_scope:true reason:ok", func() string {
			sc, failure := sealAndScopeCheck(buildAgentEvent(agentID, inScope, nil))
			if failure != "" {
				return failure
			}
			if sc == nil {
				return "scope_check missing on an ai_agent event"
			}
			if sc["in_scope"] != true {
				return fmt.Sprintf("expected in_scope:true, got %v (reason %v)", sc["in_scope"], sc["reason"])
			}
			if sc["reason"] != "ok" {
				return fmt.Sprintf("expected reason:ok, got %v", sc["reason"])
			}
			if sc["agent_external_id"] != agentID {
				return fmt.Sprintf("expected agent_external_id:%s, got %v", agentID, sc["agent_external_id"])
			}
			return ""
		})

		check(cfg, r, "out-of-scope agent action → scope_check in_scope:false reason:scope_mismatch", func() string {
			sc, failure := sealAndScopeCheck(buildAgentEvent(agentID, deniedCat, nil))
			if failure != "" {
				return failure
			}
			if sc == nil {
				return "scope_check missing on an ai_agent event"
			}
			if sc["in_scope"] != false {
				return fmt.Sprintf("expected in_scope:false, got %v", sc["in_scope"])
			}
			if sc["reason"] != "scope_mismatch" {
				return fmt.Sprintf("expected reason:scope_mismatch, got %v", sc["reason"])
			}
			return ""
		})

		check(cfg, r, "unregistered actor.id → scope_check reason:agent_not_registered", func() string {
			sc, failure := sealAndScopeCheck(buildAgentEvent(unregisteredID, inScope, nil))
			if failure != "" {
				return failure
			}
			if sc == nil {
				return "scope_check missing on an ai_agent event"
			}
			if sc["reason"] != "agent_not_registered" {
				return fmt.Sprintf("expected reason:agent_not_registered, got %v", sc["reason"])
			}
			if sc["in_scope"] != false {
				return fmt.Sprintf("expected in_scope:false, got %v", sc["in_scope"])
			}
			return ""
		})

		check(cfg, r, "non-agent event → no scope_check recorded", func() string {
			ev := buildAgentEvent(agentID, inScope, nil)
			ev["event_type"] = "data.accessed"
			ev["actor"].(map[string]any)["type"] = "user"
			sc, failure := sealAndScopeCheck(ev)
			if failure != "" {
				return failure
			}
			if sc != nil {
				return fmt.Sprintf("expected no scope_check on a non-agent event, got %v", sc)
			}
			return ""
		})
	})
}

// buildAgentEvent assembles a minimal valid ai_agent event with a controllable
// actor.id and action.category (the scope_check required scope).
func buildAgentEvent(actorID, category string, classification []string) map[string]any {
	resource := map[string]any{"type": "testsuite_resource"}
	if len(classification) > 0 {
		resource["classification"] = classification
	}
	return map[string]any{
		"event_type":  "agent.tool_called",
		"occurred_at": time.Now().UTC().Format(time.RFC3339Nano),
		"source":      map[string]any{"system": "testsuite", "service": "agent-runtime", "environment": "test"},
		"actor":       map[string]any{"type": "ai_agent", "id": actorID},
		"action":      map[string]any{"name": "testsuite_action", "category": category, "result": "success"},
		"resource":    resource,
	}
}

/* ─── http ────────────────────────────────────────────────────────────── */

var httpClient = &http.Client{Timeout: requestTimeout}

func postEvent(cfg *config, payload any, idempotencyKey string) eventResponse {
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", cfg.apiURL+"/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return eventResponse{Status: 0, Body: responseBody{Error: &errorBody{
			Code: "request_timeout", Message: err.Error(),
		}}}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	rb := responseBody{}
	_ = json.Unmarshal(raw, &rb)
	retryAfter := 0
	if ra := res.Header.Get("Retry-After"); ra != "" {
		if n, e := strconv.Atoi(ra); e == nil {
			retryAfter = n
		}
	}
	return eventResponse{Status: res.StatusCode, Body: rb, RetryAfter: retryAfter}
}

// postEventTolerant is for the functional flows (composite, chain) where a
// 429 is a budget artifact, not a failure. On free tier the suite's earlier
// sections exhaust the per-minute burst budget, so these flows would 429
// through no fault of their own. We wait out the limiter (honoring
// Retry-After, capped) and retry. Burst/race intentionally do NOT use this -
// they want to observe raw limiter behavior.
func postEventTolerant(cfg *config, payload any) eventResponse {
	const maxRetries = 2
	res := postEvent(cfg, payload, "")
	for attempt := 0; attempt < maxRetries && res.Status == 429; attempt++ {
		wait := res.RetryAfter
		if wait <= 0 {
			wait = 60 // burst window is per-minute; default to a full reset
		}
		if wait > 65 {
			wait = 65
		}
		log(cfg, fmt.Sprintf("      ↳ rate-limited; waiting %ds for window reset…", wait))
		time.Sleep(time.Duration(wait) * time.Second)
		res = postEvent(cfg, payload, "")
	}
	return res
}

func verifyEvent(cfg *config, eventID string) eventResponse {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		cfg.apiURL+"/api/v1/events/"+eventID+"/verify", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	res, err := httpClient.Do(req)
	if err != nil {
		return eventResponse{Status: 0}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	rb := responseBody{}
	_ = json.Unmarshal(raw, &rb)
	return eventResponse{Status: res.StatusCode, Body: rb}
}

func waitForSeal(cfg *config, eventID string, maxWait time.Duration) eventResponse {
	deadline := time.Now().Add(maxWait)
	var last eventResponse
	for time.Now().Before(deadline) {
		last = verifyEvent(cfg, eventID)
		if last.Body.State == "sealed" {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

// fetchScopeCheck GETs the sealed event and returns its scope_check object, or
// nil if the event carries none (non-agent / pre-registry). A non-empty string
// signals a transport/parse failure, not "absent".
func fetchScopeCheck(cfg *config, eventID string) (map[string]any, string) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", cfg.apiURL+"/api/v1/events/"+eventID, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Sprintf("GET event failed: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return nil, fmt.Sprintf("GET event returned %d", res.StatusCode)
	}
	var parsed struct {
		Event map[string]any `json:"event"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Sprintf("parse event: %v", err)
	}
	sc, _ := parsed.Event["scope_check"].(map[string]any)
	return sc, ""
}

/* ─── output helpers ──────────────────────────────────────────────────── */

func printSummary(cfg *config, r *results) {
	elapsed := time.Since(r.startedAt).Seconds()
	fmt.Printf("\n%s\n", bold(cfg, "────────────────────────────────────────"))
	pass, fail := r.pass.Load(), r.fail.Load()
	total := pass + fail
	passStr := green(cfg, fmt.Sprintf("%d pass", pass))
	failStr := fmt.Sprintf("%d fail", fail)
	if fail > 0 {
		failStr = red(cfg, failStr)
	}
	fmt.Printf("%d checks · %.2fs · %s · %s\n", total, elapsed, passStr, failStr)

	if fail > 0 {
		fmt.Printf("\n%s\n", red(cfg, bold(cfg, "Failures:")))
		r.mu.Lock()
		for _, f := range r.failures {
			fmt.Printf("  · %s\n", f)
		}
		r.mu.Unlock()
	}
}

func log(cfg *config, s string) {
	if !cfg.quiet {
		fmt.Println(s)
	}
}

func useColor(cfg *config) bool {
	if cfg.noColor {
		return false
	}
	if fi, err := os.Stdout.Stat(); err == nil {
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

func green(cfg *config, s string) string { return wrap(cfg, "\x1b[32m", s) }
func red(cfg *config, s string) string   { return wrap(cfg, "\x1b[31m", s) }
func gray(cfg *config, s string) string  { return wrap(cfg, "\x1b[90m", s) }
func bold(cfg *config, s string) string  { return wrap(cfg, "\x1b[1m", s) }

func wrap(cfg *config, code, s string) string {
	if !useColor(cfg) {
		return s
	}
	return code + s + "\x1b[0m"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// stable but order-independent: simple insertion sort
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func sortedIntKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func errBrief(e *errorBody) string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return fmt.Sprintf(`{"code":%q,"message":%q}`, e.Code, msg)
}
