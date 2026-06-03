//go:build integration

package sealer

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/stonewrit/stonewrit/classify"
	"github.com/stonewrit/stonewrit/core"
	"github.com/stonewrit/stonewrit/server/internal/migrate"
	queries "github.com/stonewrit/stonewrit/server/internal/queries/gen"
	"github.com/stonewrit/stonewrit/server/migrations"
	"github.com/stonewrit/stonewrit/spec"
)

// TestSealAndVerifyPipeline is the end-to-end proof against a real database:
// migrate the schema, write a pending event exactly as the accept path would,
// seal it, then independently verify the sealed event with the core verifier.
// It confirms the schema, the sealer, and the hashing agree.
func TestSealAndVerifyPipeline(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if _, err := migrate.Up(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	q := queries.New(pool)
	orgID := "org_" + uuid.NewString()[:8]
	projID := uuid.New()
	envID := uuid.New()
	shardIdx := int32(0)

	// Seed tenancy directly (the dashboard owns these in production).
	if _, err := pool.Exec(ctx,
		`INSERT INTO projects (id, organization_id, name, slug, created_by_user_id)
		 VALUES ($1, $2, 'test', $3, 'user_test')`,
		projID, orgID, "proj-"+uuid.NewString()[:8]); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO environments (id, organization_id, project_id, name, slug, type)
		 VALUES ($1, $2, $3, 'prod', $4, 'production')`,
		envID, orgID, projID, "env-"+uuid.NewString()[:8]); err != nil {
		t.Fatalf("insert environment: %v", err)
	}

	// Build a pending event exactly as the accept path does.
	receivedAt := time.Now().UTC().Truncate(time.Millisecond)
	extID := "ext-" + uuid.NewString()
	event := spec.EventInput{
		EventType:       "data.accessed",
		OccurredAt:      receivedAt,
		ExternalEventID: extID,
		Source:          spec.Source{System: "billing", Service: "api"},
		Actor:           spec.Actor{Type: "human", ID: "u1"},
		Action:          spec.Action{Name: "read", Category: "data", Result: "allowed"},
		Resource:        spec.Resource{Type: "invoice", Classification: []string{"pii"}},
	}
	payloadHash, err := core.PayloadHash(core.BuildContentPayload(core.ContentInput{
		OrganizationID: orgID,
		ProjectID:      projID.String(),
		EnvironmentID:  envID.String(),
		ReceivedAt:     receivedAt,
		Event:          event,
	}))
	if err != nil {
		t.Fatalf("payload hash: %v", err)
	}

	eventData, _ := json.Marshal(eventDataEnvelope{ReceivedAt: receivedAt, Input: event})
	cl := classify.Classify(classify.Event{
		EventType: event.EventType, ActorType: event.Actor.Type,
		ActionResult: event.Action.Result, DataClasses: event.Resource.Classification,
	})
	classificationJSON, _ := json.Marshal(cl)
	mappingsJSON, _ := json.Marshal([]classify.Mapping{})

	eventID := uuid.New()
	if err := q.InsertPendingEvent(ctx, queries.InsertPendingEventParams{
		ID:               pgtype.UUID{Bytes: eventID, Valid: true},
		OrganizationID:   orgID,
		ProjectID:        pgtype.UUID{Bytes: projID, Valid: true},
		EnvironmentID:    pgtype.UUID{Bytes: envID, Valid: true},
		ShardIndex:       shardIdx,
		ExternalEventID:  &extID,
		PayloadHash:      payloadHash,
		EventData:        eventData,
		Classification:   classificationJSON,
		ControlMappings:  mappingsJSON,
		ApiKeyMetadataID: pgtype.UUID{Bytes: uuid.New(), Valid: true},
	}); err != nil {
		t.Fatalf("insert pending: %v", err)
	}

	// Seal the shard.
	s := &Sealer{Pool: pool, Q: q, Log: zerolog.Nop()}
	s.Defaults()
	if err := s.SealShard(ctx, pgtype.UUID{Bytes: envID, Valid: true}, shardIdx); err != nil {
		t.Fatalf("seal shard: %v", err)
	}

	// Read the sealed event and verify it independently with core.
	ev, err := q.GetEventForOrg(ctx, queries.GetEventForOrgParams{
		ID:             pgtype.UUID{Bytes: eventID, Valid: true},
		OrganizationID: orgID,
	})
	if err != nil {
		t.Fatalf("get sealed event: %v", err)
	}
	if ev.ChainPosition != 1 {
		t.Errorf("expected chain position 1, got %d", ev.ChainPosition)
	}
	if ev.PayloadHash != payloadHash {
		t.Errorf("stored payload hash differs:\n stored:  %s\n compute: %s", ev.PayloadHash, payloadHash)
	}

	var stored spec.EventInput
	if err := json.Unmarshal(ev.RawPayload, &stored); err != nil {
		t.Fatalf("unmarshal raw_payload: %v", err)
	}
	res := core.VerifyEvent(core.Record{
		PreviousEventHash: ev.PreviousEventHash,
		PayloadHash:       ev.PayloadHash,
		EventHash:         ev.EventHash,
		ChainPosition:     ev.ChainPosition,
		Content: &core.ContentInput{
			OrganizationID: ev.OrganizationID,
			ProjectID:      uuid.UUID(ev.ProjectID.Bytes).String(),
			EnvironmentID:  uuid.UUID(ev.EnvironmentID.Bytes).String(),
			ReceivedAt:     ev.ReceivedAt.Time,
			Event:          stored,
		},
	})
	if !res.Valid {
		t.Errorf("core verification of sealed event failed: %+v", res)
	}
}
