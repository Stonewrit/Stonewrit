-- 0001_core_ingest: the schema the open ingest server owns.
--
-- This is the full schema for the self-hosted server: the tables it writes and
-- reads to accept events, seal them into a tamper-evident hash chain, classify
-- them against a compliance catalog, and export evidence. Point the server at
-- any Postgres and run `stonewrit migrate up` once.
--
-- There are no users, organizations, subscriptions, or billing here. This is an
-- open, self-hosted server. organization_id survives only as a namespace string
-- (it is part of the frozen content hash) and defaults to 'default'. Everything
-- the server mints is a uuid.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Tenancy ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS projects (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL DEFAULT 'default',
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

CREATE TABLE IF NOT EXISTS environments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL DEFAULT 'default',
    project_id      uuid NOT NULL REFERENCES projects (id),
    name            text NOT NULL,
    slug            text NOT NULL,
    type            text NOT NULL,
    region          text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);

-- API keys --------------------------------------------------------------------
-- Created and revoked with the `stonewrit key` CLI. The token itself is never
-- stored; only its SHA-256 hex digest. Auth is opt-in (APIKEY_AUTH=true).

CREATE TABLE IF NOT EXISTS api_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash        text NOT NULL UNIQUE,
    name            text NOT NULL,
    organization_id text NOT NULL DEFAULT 'default',
    project_id      uuid NOT NULL REFERENCES projects (id),
    environment_id  uuid NOT NULL REFERENCES environments (id),
    scopes          text[] NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    revoked_at      timestamptz
);

-- Compliance catalog ----------------------------------------------------------
-- Seeded idempotently on server boot from the embedded baseline ruleset.

CREATE TABLE IF NOT EXISTS frameworks (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    description text,
    version     text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS controls (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    framework   text NOT NULL,
    control_id  text NOT NULL,
    title       text NOT NULL,
    description text,
    category    text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (framework, control_id)
);

-- Chains ----------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS chains (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   text NOT NULL DEFAULT 'default',
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    name              text NOT NULL,
    status            text NOT NULL DEFAULT 'active',
    latest_position   bigint NOT NULL DEFAULT 0,
    latest_event_hash text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    shard_index       integer NOT NULL DEFAULT 0,
    UNIQUE (environment_id, shard_index)
);

CREATE TABLE IF NOT EXISTS chain_checkpoints (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  text NOT NULL DEFAULT 'default',
    project_id       uuid NOT NULL,
    environment_id   uuid NOT NULL,
    chain_id         uuid NOT NULL,
    from_position    bigint NOT NULL,
    to_position      bigint NOT NULL,
    root_hash        text NOT NULL,
    event_count      integer NOT NULL,
    anchoring_type   text NOT NULL DEFAULT 'internal',
    anchoring_status text NOT NULL DEFAULT 'completed',
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- Pending events (the accept-path write, drained by the sealer) ---------------

CREATE TABLE IF NOT EXISTS pending_events (
    id                uuid PRIMARY KEY,
    organization_id   text NOT NULL DEFAULT 'default',
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    shard_index       integer NOT NULL,
    external_event_id text,
    payload_hash      text NOT NULL,
    event_data        jsonb NOT NULL,
    classification    jsonb NOT NULL,
    control_mappings  jsonb NOT NULL,
    api_key_id        uuid NOT NULL,
    idempotency_key   text,
    accepted_at       timestamptz NOT NULL DEFAULT now(),
    -- scope_check is last so a column-list read returns the model verbatim.
    scope_check       jsonb
);

CREATE INDEX IF NOT EXISTS pending_events_shard
    ON pending_events (environment_id, shard_index, accepted_at, id);

-- Events (the immutable, hash-chained journal) --------------------------------

CREATE TABLE IF NOT EXISTS events (
    id                       uuid PRIMARY KEY,
    organization_id          text NOT NULL DEFAULT 'default',
    project_id               uuid NOT NULL,
    environment_id           uuid NOT NULL,
    external_event_id        text,
    event_type               text NOT NULL,
    occurred_at              timestamptz NOT NULL,
    received_at              timestamptz NOT NULL,
    source_system            text NOT NULL,
    source_service           text NOT NULL,
    source_environment       text,
    source_region            text,
    source_version           text,
    actor_type               text NOT NULL,
    actor_id                 text,
    actor_id_hash            text,
    actor_email              text,
    actor_role               text,
    human_supervisor_id      text,
    action_name              text NOT NULL,
    action_category          text NOT NULL,
    action_result            text NOT NULL,
    action_reason            text,
    resource_type            text NOT NULL,
    resource_id              text,
    resource_id_hash         text,
    tenant_id                text,
    tenant_id_hash           text,
    data_classes             text[] NOT NULL DEFAULT '{}',
    policy_decision          text,
    policy_id                text,
    approval_required        boolean,
    approval_id              text,
    request_id               text,
    trace_id                 text,
    session_id_hash          text,
    raw_payload              jsonb NOT NULL,
    metadata                 jsonb,
    chain_id                 uuid NOT NULL,
    chain_position           bigint NOT NULL,
    payload_hash             text NOT NULL,
    previous_event_hash      text,
    event_hash               text NOT NULL,
    hash_algorithm           text NOT NULL,
    canonicalization_version text NOT NULL,
    classification_status    text NOT NULL DEFAULT 'completed',
    created_at               timestamptz NOT NULL DEFAULT now(),
    sealing_status           text NOT NULL DEFAULT 'sealed',
    redacted_at              timestamptz,
    scope_check_result       jsonb
);

-- Dedup invariant the sealer relies on: one external_event_id per environment.
-- NULL external ids are allowed and never collide.
CREATE UNIQUE INDEX IF NOT EXISTS events_environment_external_event_id
    ON events (environment_id, external_event_id)
    WHERE external_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS events_chain_position ON events (chain_id, chain_position);
CREATE INDEX IF NOT EXISTS events_org_project_received ON events (organization_id, project_id, received_at);

CREATE TABLE IF NOT EXISTS event_classifications (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL DEFAULT 'default',
    event_id        uuid NOT NULL,
    data_classes    text[] NOT NULL DEFAULT '{}',
    risk_level      text,
    confidence      numeric,
    method          text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS event_control_mappings (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL DEFAULT 'default',
    event_id        uuid NOT NULL,
    framework       text NOT NULL,
    control_id      text NOT NULL,
    mapping_reason  text,
    confidence      numeric,
    mapped_by       text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    status          text NOT NULL DEFAULT 'auto'
);

CREATE INDEX IF NOT EXISTS event_control_mappings_event ON event_control_mappings (organization_id, event_id);

-- Idempotency -----------------------------------------------------------------

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL DEFAULT 'default',
    project_id      uuid NOT NULL,
    environment_id  uuid NOT NULL,
    api_key_id      uuid NOT NULL,
    key             text NOT NULL,
    request_hash    text NOT NULL,
    response_body   bytea NOT NULL,
    status_code     integer,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    UNIQUE (organization_id, key)
);

-- Evidence + exceptions -------------------------------------------------------

CREATE TABLE IF NOT EXISTS evidence_exports (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     text NOT NULL DEFAULT 'default',
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    type                text NOT NULL,
    framework           text,
    period_from         timestamptz,
    period_to           timestamptz,
    controls            text[],
    format              text NOT NULL DEFAULT 'json',
    include_raw_events  boolean,
    include_hash_proofs boolean,
    status              text NOT NULL DEFAULT 'processing',
    file_url            text,
    error_message       text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    completed_at        timestamptz,
    bundle              jsonb,
    bundle_size         integer,
    event_count         integer,
    bundle_hash         text,
    bundle_signed_at    timestamptz,
    agent_external_id   text,
    data_classes_filter text[]
);

CREATE TABLE IF NOT EXISTS exceptions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   text NOT NULL DEFAULT 'default',
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    title             text NOT NULL,
    description       text,
    severity          text NOT NULL,
    status            text NOT NULL,
    opened_at         timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz,
    resolved_at       timestamptz,
    related_event_ids uuid[],
    control_ids       text[],
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Agent registry (read-only; optional) ----------------------------------------
-- Empty by default. When present, an agent event's actor.id resolves here to
-- compute an observe-only scope check. Populating it is optional.

CREATE TABLE IF NOT EXISTS agents (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   text NOT NULL DEFAULT 'default',
    project_id        uuid NOT NULL,
    name              text NOT NULL,
    external_id       text NOT NULL,
    authorized_scopes text[] NOT NULL DEFAULT '{}',
    status            text NOT NULL DEFAULT 'active',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, project_id, external_id)
);
