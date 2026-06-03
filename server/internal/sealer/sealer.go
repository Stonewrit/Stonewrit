// Package sealer drains pending_events into the immutable events
// journal, assigning chain positions + computing chain hashes under a
// per-shard SELECT FOR UPDATE lock.
//
// One sealer process per Go instance is fine; multiple processes can
// run safely because the shard lock serialises them at the Postgres
// level. With N shards per environment, you get N parallel sealers
// per env across the whole fleet without coordination.
//
// Loop:
//  1. Find shards with pending rows (ListShardsWithPending)
//  2. For each shard: BEGIN; lock chain row; drain up to BatchSize
//     pending rows; compute hashes; pgx.CopyFrom into events;
//     insert classifications + control_mappings; update chain tip;
//     delete drained rows; COMMIT.
//
// Target: <500ms p99 from pending insert to seal commit at steady state.
package sealer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/classify"
	"github.com/stonewrit/stonewrit/core"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/spec"
)

type Sealer struct {
	Pool       *pgxpool.Pool
	Q          *queries.Queries
	Log        zerolog.Logger
	BatchSize  int32
	PollPeriod time.Duration
	MaxWork    int32
	// Max shards sealed in parallel per tick (bounded worker pool). Default 8.
	Concurrency int
	// Optional external anchor (S3 Object Lock / OpenTimestamps / etc.). When
	// nil, checkpoints are internal-only (still a dangling in-DB proof). Wiring
	// a real implementation is a fast-follow; the interface keeps that pluggable.
	Anchorer Anchorer
}

// Checkpoint is the minimal chain-head reference handed to an external Anchorer.
type Checkpoint struct {
	ChainID    string
	ToPosition int64
	RootHash   string
}

// Anchorer publishes a chain checkpoint to an external append-only store so the
// proof survives a full DB compromise. Implementations must be best-effort and
// non-fatal to ingest/sealing.
type Anchorer interface {
	PublishCheckpoint(ctx context.Context, cp Checkpoint)
}

func (s *Sealer) Defaults() {
	if s.BatchSize == 0 {
		s.BatchSize = 1000
	}
	if s.PollPeriod == 0 {
		s.PollPeriod = 100 * time.Millisecond
	}
	if s.MaxWork == 0 {
		s.MaxWork = 256
	}
	if s.Concurrency == 0 {
		// Seal shards in parallel - serial sealing can't keep up under load.
		// Bounded so concurrent batch txns don't exhaust the pgx pool; keep
		// well under DATABASE_POOL_MAX.
		s.Concurrency = 8
	}
}

func (s *Sealer) Run(ctx context.Context) error {
	s.Defaults()
	s.Log.Info().
		Int32("batch_size", s.BatchSize).
		Dur("poll_period", s.PollPeriod).
		Msg("sealer running")

	ticker := time.NewTicker(s.PollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.Log.Info().Msg("sealer stopping")
			return nil
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Sealer) tick(ctx context.Context) {
	shards, err := s.Q.ListShardsWithPending(ctx, s.MaxWork)
	if err != nil {
		s.Log.Error().Err(err).Msg("list shards with pending failed")
		return
	}
	if len(shards) == 0 {
		return
	}

	// Seal shards concurrently, bounded by Concurrency. Distinct shards never
	// conflict (each holds its own chain row FOR UPDATE); a shard already being
	// worked by another goroutine/process is skipped via SKIP LOCKED. This is
	// the throughput multiplier - serial sealing falls behind under real load.
	sem := make(chan struct{}, s.Concurrency)
	var wg sync.WaitGroup
	for _, sh := range shards {
		sh := sh
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.SealShard(ctx, sh.EnvironmentID, sh.ShardIndex); err != nil {
				s.Log.Error().
					Err(err).
					Str("env_id", sh.EnvironmentID.String()).
					Int32("shard_index", sh.ShardIndex).
					Msg("seal shard failed")
			}
		}()
	}
	wg.Wait()
}

// SealShard atomically drains one shard's pending events into events.
// Holds the chain row's FOR UPDATE lock for the entire batch so no other
// sealer can interleave positions on the same chain.
func (s *Sealer) SealShard(ctx context.Context, envID pgtype.UUID, shardIdx int32) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.Q.WithTx(tx)

	// Create the shard chain if missing (no lock taken on the existing row).
	if err := qtx.EnsureShardChainFromEnv(ctx, queries.EnsureShardChainFromEnvParams{
		ID:      envID,
		Column2: shardIdx,
	}); err != nil {
		return fmt.Errorf("ensure chain: %w", err)
	}

	// Claim the chain row FOR UPDATE SKIP LOCKED. No row → another sealer is
	// already working this shard; skip it this tick (commit the empty tx and
	// move on) instead of blocking the whole pool on it.
	locked, err := qtx.LockShardChain(ctx, queries.LockShardChainParams{
		EnvironmentID: envID,
		ShardIndex:    shardIdx,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("lock chain: %w", err)
	}

	pending, err := qtx.DrainPendingForShard(ctx, queries.DrainPendingForShardParams{
		EnvironmentID: envID,
		ShardIndex:    shardIdx,
		Limit:         s.BatchSize,
	})
	if err != nil {
		return fmt.Errorf("drain pending: %w", err)
	}
	if len(pending) == 0 {
		return tx.Commit(ctx)
	}

	// De-duplicate before COPY. The events table has a uniqueness invariant
	// on (environment_id, external_event_id); if even one row collides, the
	// atomic COPY fails and the shard wedges forever. Two sources of
	// collision to handle:
	//   1. another sealer batch already promoted an event with the same
	//      external_event_id (race window during accept).
	//   2. duplicates within the *current* batch (same external_event_id
	//      enqueued twice by a racing accept call).
	// We still DELETE the dropped pending rows so they don't recycle.
	keepIdx, droppedIDs, err := s.dedupePending(ctx, qtx, envID, pending)
	if err != nil {
		return err
	}
	kept := make([]queries.PendingEvent, 0, len(keepIdx))
	for _, i := range keepIdx {
		kept = append(kept, pending[i])
	}
	if len(kept) == 0 {
		// Everything was a duplicate. Delete the pending rows and exit so we
		// don't loop on the same data next tick.
		allIDs := append(droppedIDs, pendingIDs(pending)...)
		if err := qtx.DeletePendingByIds(ctx, dedupeUUIDs(allIDs)); err != nil {
			return fmt.Errorf("delete drained pending (all dupes): %w", err)
		}
		return tx.Commit(ctx)
	}

	prevHash := ""
	if locked.LatestEventHash != nil {
		prevHash = *locked.LatestEventHash
	}
	position := locked.LatestPosition + 1
	startPos := position

	rows := make([][]any, 0, len(kept))
	classifications := make([]classify.Result, len(kept))
	mappings := make([][]classify.Mapping, len(kept))

	for i, p := range kept {
		var env eventDataEnvelope
		if err := json.Unmarshal(p.EventData, &env); err != nil {
			return fmt.Errorf("decode pending event_data[%d]: %w", i, err)
		}

		if err := json.Unmarshal(p.Classification, &classifications[i]); err != nil {
			return fmt.Errorf("decode classification[%d]: %w", i, err)
		}
		if err := json.Unmarshal(p.ControlMappings, &mappings[i]); err != nil {
			return fmt.Errorf("decode control_mappings[%d]: %w", i, err)
		}

		eventHash := core.EventHash(prevHash, p.PayloadHash, position)
		row, err := buildEventRow(p, env, locked.ID, locked.ProjectID, position, prevHash, eventHash, classifications[i])
		if err != nil {
			return fmt.Errorf("build event row[%d]: %w", i, err)
		}
		rows = append(rows, row)

		prevHash = eventHash
		position++
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"events"}, eventColumns, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy events: %w", err)
	}

	// Classifications + control_mappings - bulk COPY instead of a per-row INSERT
	// loop. A 1000-event batch was ~1000 classification inserts + N mapping
	// inserts serially inside the tx; two CopyFroms collapse that to two round
	// trips, which is the dominant per-batch cost under real load.
	classRows := make([][]any, 0, len(kept))
	mapRows := make([][]any, 0, len(kept))
	for i, p := range kept {
		var classConf pgtype.Numeric
		_ = classConf.Scan("1.0")
		classRows = append(classRows, []any{
			p.OrganizationID,
			p.ID,
			classifications[i].DataClasses,
			strPtr(string(classifications[i].RiskLevel)),
			classConf,
			"rules",
		})
		for _, m := range mappings[i] {
			var mapConf pgtype.Numeric
			_ = mapConf.Scan("0.8")
			mapRows = append(mapRows, []any{
				p.OrganizationID,
				p.ID,
				m.Framework,
				m.ControlID,
				strPtr(m.Reason),
				mapConf,
				"rules",
				"auto",
			})
		}
	}

	if len(classRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"event_classifications"},
			classificationColumns, pgx.CopyFromRows(classRows)); err != nil {
			return fmt.Errorf("copy classifications: %w", err)
		}
	}
	if len(mapRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"event_control_mappings"},
			controlMappingColumns, pgx.CopyFromRows(mapRows)); err != nil {
			return fmt.Errorf("copy control mappings: %w", err)
		}
	}

	if err := qtx.UpdateShardTip(ctx, queries.UpdateShardTipParams{
		ID:              locked.ID,
		LatestPosition:  position - 1,
		LatestEventHash: &prevHash,
	}); err != nil {
		return fmt.Errorf("update chain tip: %w", err)
	}

	// Anchor: record a checkpoint for this batch (root_hash = the new tip
	// event_hash, which commits to the whole chain). Populates chain_checkpoints
	// so deletion leaves a dangling proof and an external Anchorer can publish
	// the root. In the same tx as the seal so it can't drift from the tip.
	if err := qtx.InsertChainCheckpoint(ctx, queries.InsertChainCheckpointParams{
		OrganizationID: locked.OrganizationID,
		ProjectID:      locked.ProjectID,
		EnvironmentID:  envID,
		ChainID:        locked.ID,
		FromPosition:   startPos,
		ToPosition:     position - 1,
		RootHash:       prevHash,
		EventCount:     int32(len(kept)),
	}); err != nil {
		return fmt.Errorf("insert checkpoint: %w", err)
	}
	if s.Anchorer != nil {
		// Best-effort external publish (S3 Object Lock / OTS / etc.). A failure
		// must not roll back the seal - the internal checkpoint already exists;
		// re-anchoring is the Anchorer's responsibility.
		s.Anchorer.PublishCheckpoint(ctx, Checkpoint{
			ChainID:    locked.ID.String(),
			ToPosition: position - 1,
			RootHash:   prevHash,
		})
	}

	// Delete ALL drained pending rows - including the duplicates we
	// dropped. They're already represented in the journal by the original
	// winning event.
	allIDs := append(pendingIDs(pending), droppedIDs...)
	if err := qtx.DeletePendingByIds(ctx, dedupeUUIDs(allIDs)); err != nil {
		return fmt.Errorf("delete drained pending: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	s.Log.Debug().
		Int("count", len(pending)).
		Int64("start_position", startPos).
		Int64("end_position", position-1).
		Str("chain_id", locked.ID.String()).
		Msg("shard sealed")
	return nil
}

type eventDataEnvelope struct {
	ReceivedAt time.Time       `json:"received_at"`
	Input      spec.EventInput `json:"input"`
}

// classificationColumns / controlMappingColumns must match the value order in
// the SealShard CopyFrom row builders (and the INSERT queries in events.sql).
var classificationColumns = []string{
	"organization_id",
	"event_id",
	"data_classes",
	"risk_level",
	"confidence",
	"method",
}

var controlMappingColumns = []string{
	"organization_id",
	"event_id",
	"framework",
	"control_id",
	"mapping_reason",
	"confidence",
	"mapped_by",
	"status",
}

// eventColumns must match buildEventRow's order exactly. Keep in sync.
var eventColumns = []string{
	"id",
	"organization_id",
	"project_id",
	"environment_id",
	"external_event_id",
	"event_type",
	"occurred_at",
	"received_at",
	"source_system",
	"source_service",
	"source_environment",
	"source_region",
	"source_version",
	"actor_type",
	"actor_id",
	"actor_id_hash",
	"actor_email",
	"actor_role",
	"human_supervisor_id",
	"action_name",
	"action_category",
	"action_result",
	"action_reason",
	"resource_type",
	"resource_id",
	"resource_id_hash",
	"tenant_id",
	"tenant_id_hash",
	"data_classes",
	"policy_decision",
	"policy_id",
	"approval_required",
	"approval_id",
	"request_id",
	"trace_id",
	"session_id_hash",
	"raw_payload",
	"metadata",
	"chain_id",
	"chain_position",
	"payload_hash",
	"previous_event_hash",
	"event_hash",
	"hash_algorithm",
	"canonicalization_version",
	"classification_status",
	"sealing_status",
	"scope_check_result",
}

func buildEventRow(
	p queries.PendingEvent,
	env eventDataEnvelope,
	chainID pgtype.UUID,
	projectID pgtype.UUID,
	position int64,
	prevHash string,
	eventHash string,
	cl classify.Result,
) ([]any, error) {
	input := env.Input
	rawPayload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var metaBytes []byte
	if input.Metadata != nil {
		metaBytes, err = json.Marshal(input.Metadata)
		if err != nil {
			return nil, err
		}
	}

	var prev any
	if prevHash != "" {
		prev = prevHash
	}

	var policyDecision, policyID, approvalID any
	var approvalRequired any = false
	if input.Policy != nil {
		if input.Policy.Decision != "" {
			policyDecision = input.Policy.Decision
		}
		if input.Policy.PolicyID != "" {
			policyID = input.Policy.PolicyID
		}
		if input.Policy.ApprovalID != "" {
			approvalID = input.Policy.ApprovalID
		}
		if input.Policy.ApprovalRequired != nil {
			approvalRequired = *input.Policy.ApprovalRequired
		}
	}

	var reqID, traceID, sessionHash any
	if input.Request != nil {
		if input.Request.RequestID != "" {
			reqID = input.Request.RequestID
		}
		if input.Request.TraceID != "" {
			traceID = input.Request.TraceID
		}
		if input.Request.SessionIDHash != "" {
			sessionHash = input.Request.SessionIDHash
		}
	}

	dataClasses := cl.DataClasses
	if dataClasses == nil {
		dataClasses = []string{}
	}

	return []any{
		p.ID,
		p.OrganizationID,
		projectID,
		p.EnvironmentID,
		nullIfEmpty(input.ExternalEventID),
		input.EventType,
		input.OccurredAt.UTC(),
		env.ReceivedAt,
		input.Source.System,
		input.Source.Service,
		nullIfEmpty(input.Source.Environment),
		nullIfEmpty(input.Source.Region),
		nullIfEmpty(input.Source.Version),
		input.Actor.Type,
		nullIfEmpty(input.Actor.ID),
		nullIfEmpty(input.Actor.IDHash),
		nullIfEmpty(input.Actor.Email),
		nullIfEmpty(input.Actor.Role),
		nullIfEmpty(input.Actor.HumanSupervisorID),
		input.Action.Name,
		input.Action.Category,
		input.Action.Result,
		nullIfEmpty(input.Action.Reason),
		input.Resource.Type,
		nullIfEmpty(input.Resource.ID),
		nullIfEmpty(input.Resource.IDHash),
		nullIfEmpty(input.Resource.TenantID),
		nullIfEmpty(input.Resource.TenantIDHash),
		dataClasses,
		policyDecision,
		policyID,
		approvalRequired,
		approvalID,
		reqID,
		traceID,
		sessionHash,
		rawPayload,
		nullIfEmptyBytes(metaBytes),
		chainID,
		position,
		p.PayloadHash,
		prev,
		eventHash,
		"sha256",
		"jcs-v1",
		"completed",
		"sealed",
		// scope_check_result: copied verbatim from pending_events.scope_check
		// (computed observe-only at accept). NULL for non-agent / pre-registry
		// events. Never recomputed here - the accept-time snapshot is canonical.
		nullIfEmptyBytes(p.ScopeCheck),
	}, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmptyBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// dedupePending walks the batch and decides which rows actually go into
// the events table this tick.
//
// A pending row is dropped (but still scheduled for deletion) when:
//   - its external_event_id already exists in the events table for this
//     env (some other batch already promoted that event), OR
//   - an earlier row in the same batch already has that external_event_id
//     (the race condition during accept inserted duplicate pendings).
//
// Returns:
//   - keepIdx: indices into `pending` to KEEP for the batch insert.
//   - droppedIDs: pending.id values that should be deleted alongside the
//     kept ones at the end of the transaction.
func (s *Sealer) dedupePending(
	ctx context.Context,
	qtx *queries.Queries,
	envID pgtype.UUID,
	pending []queries.PendingEvent,
) (keepIdx []int, droppedIDs []pgtype.UUID, err error) {
	// Collect all non-null external_event_ids in the batch.
	candidates := make([]string, 0, len(pending))
	for _, p := range pending {
		if p.ExternalEventID != nil && *p.ExternalEventID != "" {
			candidates = append(candidates, *p.ExternalEventID)
		}
	}

	already := make(map[string]struct{}, len(candidates))
	if len(candidates) > 0 {
		existing, err := qtx.ExistingExternalEventIDs(ctx, queries.ExistingExternalEventIDsParams{
			EnvironmentID: envID,
			Column2:       candidates,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("look up existing external_event_ids: %w", err)
		}
		for _, e := range existing {
			if e != nil {
				already[*e] = struct{}{}
			}
		}
	}

	seenInBatch := make(map[string]struct{}, len(candidates))
	keepIdx = make([]int, 0, len(pending))
	droppedIDs = make([]pgtype.UUID, 0)

	for i, p := range pending {
		// Rows without an external_event_id never collide.
		if p.ExternalEventID == nil || *p.ExternalEventID == "" {
			keepIdx = append(keepIdx, i)
			continue
		}
		extID := *p.ExternalEventID
		if _, dup := already[extID]; dup {
			droppedIDs = append(droppedIDs, p.ID)
			s.Log.Warn().
				Str("external_event_id", extID).
				Str("pending_id", uuid.UUID(p.ID.Bytes).String()).
				Msg("dropping pending row: external_event_id already in events")
			continue
		}
		if _, dup := seenInBatch[extID]; dup {
			droppedIDs = append(droppedIDs, p.ID)
			s.Log.Warn().
				Str("external_event_id", extID).
				Str("pending_id", uuid.UUID(p.ID.Bytes).String()).
				Msg("dropping pending row: duplicate external_event_id within batch")
			continue
		}
		seenInBatch[extID] = struct{}{}
		keepIdx = append(keepIdx, i)
	}
	return keepIdx, droppedIDs, nil
}

func pendingIDs(pending []queries.PendingEvent) []pgtype.UUID {
	out := make([]pgtype.UUID, len(pending))
	for i, p := range pending {
		out[i] = p.ID
	}
	return out
}

func dedupeUUIDs(in []pgtype.UUID) []pgtype.UUID {
	seen := make(map[uuid.UUID]struct{}, len(in))
	out := make([]pgtype.UUID, 0, len(in))
	for _, u := range in {
		k := uuid.UUID(u.Bytes)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, u)
	}
	return out
}

var _ = errors.Is
