package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/core"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
)

type GenerateExport struct {
	Pool       *pgxpool.Pool
	Q          *queries.Queries
	Log        zerolog.Logger
	PollPeriod time.Duration // default 1s
}

func (w *GenerateExport) Run(ctx context.Context) error {
	if w.PollPeriod == 0 {
		w.PollPeriod = 1 * time.Second
	}
	w.Log.Info().
		Dur("poll_period", w.PollPeriod).
		Msg("generate_export worker running")

	ticker := time.NewTicker(w.PollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.processOne(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				w.Log.Error().Err(err).Msg("export worker tick failed")
			}
		}
	}
}

func (w *GenerateExport) processOne(ctx context.Context) error {
	tx, err := w.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := w.Q.WithTx(tx)
	job, err := qtx.ClaimPendingExport(ctx)
	if err != nil {
		return err
	}

	exportID := uuid.UUID(job.ID.Bytes).String()
	w.Log.Info().Str("export_id", exportID).Msg("building evidence bundle")

	if err := w.buildAndCommit(ctx, qtx, job); err != nil {
		// Best effort: write failure inside same tx so the row reflects state.
		_ = qtx.MarkExportFailed(ctx, queries.MarkExportFailedParams{
			ID:           job.ID,
			ErrorMessage: strPtr(err.Error()),
		})
		_ = tx.Commit(ctx)
		w.Log.Error().Err(err).Str("export_id", exportID).Msg("export failed")
		return nil
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit export: %w", err)
	}
	w.Log.Info().Str("export_id", exportID).Msg("export completed")
	return nil
}

type bundle struct {
	SchemaVersion string         `json:"schema_version"`
	Format        string         `json:"format"`
	ExportID      string         `json:"export_id"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Scope         bundleScope    `json:"scope"`
	Chains        []bundleChain  `json:"chains"`
	Events        []bundleEvent  `json:"events"`
	Totals        map[string]any `json:"totals"`
}

type bundleScope struct {
	OrganizationID string    `json:"organization_id"`
	ProjectID      string    `json:"project_id"`
	Framework      *string   `json:"framework,omitempty"`
	PeriodFrom     time.Time `json:"period_from"`
	PeriodTo       time.Time `json:"period_to"`
}

type bundleChain struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environment_id"`
	// No shard_index: sharding is an internal write-throughput detail. Auditors
	// group chains by environment_id and verify each chain's tip; they don't
	// need to know which are shards. Matches the Next.js bundle shape.
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	LatestPosition  int64   `json:"latest_position"`
	LatestEventHash *string `json:"latest_event_hash,omitempty"`
}

type bundleEvent struct {
	ID            string    `json:"id"`
	EventType     string    `json:"event_type"`
	OccurredAt    time.Time `json:"occurred_at"`
	ReceivedAt    time.Time `json:"received_at"`
	ActorType     string    `json:"actor_type"`
	ActorID       *string   `json:"actor_id,omitempty"`
	ActionName    string    `json:"action_name"`
	ActionResult  string    `json:"action_result"`
	ResourceType  string    `json:"resource_type"`
	ResourceID    *string   `json:"resource_id,omitempty"`
	ChainID       string    `json:"chain_id"`
	ChainPosition int64     `json:"chain_position"`
	PayloadHash   string    `json:"payload_hash"`
	EventHash     string    `json:"event_hash"`
	DataClasses   []string  `json:"data_classes,omitempty"`
}

func (w *GenerateExport) buildAndCommit(ctx context.Context, qtx *queries.Queries, job queries.ClaimPendingExportRow) error {
	events, err := qtx.ListEventsForExport(ctx, queries.ListEventsForExportParams{
		OrganizationID: job.OrganizationID,
		ProjectID:      job.ProjectID,
		ReceivedAt:     job.PeriodFrom,
		ReceivedAt_2:   job.PeriodTo,
	})
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	chains, err := qtx.ListChainSummariesForOrg(ctx, queries.ListChainSummariesForOrgParams{
		OrganizationID: job.OrganizationID,
		ProjectID:      job.ProjectID,
	})
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}

	b := bundle{
		SchemaVersion: "stonewrit-evidence-bundle/1",
		Format:        "json",
		ExportID:      uuid.UUID(job.ID.Bytes).String(),
		GeneratedAt:   time.Now().UTC(),
		Scope: bundleScope{
			OrganizationID: job.OrganizationID,
			ProjectID:      uuid.UUID(job.ProjectID.Bytes).String(),
			Framework:      job.Framework,
			PeriodFrom:     job.PeriodFrom.Time,
			PeriodTo:       job.PeriodTo.Time,
		},
	}

	for _, c := range chains {
		b.Chains = append(b.Chains, bundleChain{
			ID:              uuid.UUID(c.ID.Bytes).String(),
			EnvironmentID:   uuid.UUID(c.EnvironmentID.Bytes).String(),
			Name:            c.Name,
			Status:          c.Status,
			LatestPosition:  c.LatestPosition,
			LatestEventHash: c.LatestEventHash,
		})
	}
	for _, e := range events {
		b.Events = append(b.Events, bundleEvent{
			ID:            uuid.UUID(e.ID.Bytes).String(),
			EventType:     e.EventType,
			OccurredAt:    e.OccurredAt.Time,
			ReceivedAt:    e.ReceivedAt.Time,
			ActorType:     e.ActorType,
			ActorID:       e.ActorID,
			ActionName:    e.ActionName,
			ActionResult:  e.ActionResult,
			ResourceType:  e.ResourceType,
			ResourceID:    e.ResourceID,
			ChainID:       uuid.UUID(e.ChainID.Bytes).String(),
			ChainPosition: e.ChainPosition,
			PayloadHash:   e.PayloadHash,
			EventHash:     e.EventHash,
			DataClasses:   e.DataClasses,
		})
	}
	b.Totals = map[string]any{
		"event_count": len(events),
		"chain_count": len(chains),
	}

	// Sign: JCS(bundle) → SHA-256 → "sha256:" + hex.
	canon, err := core.CanonicalJSON(b)
	if err != nil {
		return fmt.Errorf("canonicalize: %w", err)
	}
	sum := sha256.Sum256(canon)
	bundleHash := "sha256:" + hex.EncodeToString(sum[:])

	// Store bundle as the canonical JSON bytes so verifiers re-canonicalize
	// over the same input.
	var stored json.RawMessage = canon

	sizeInt32 := int32(len(canon))
	countInt32 := int32(len(events))
	if err := qtx.MarkExportCompleted(ctx, queries.MarkExportCompletedParams{
		ID:         job.ID,
		Bundle:     stored,
		BundleSize: &sizeInt32,
		BundleHash: &bundleHash,
		EventCount: &countInt32,
	}); err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	return nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
