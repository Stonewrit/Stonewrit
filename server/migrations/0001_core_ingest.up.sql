-- 0001_core_ingest: the schema the open ingest server owns.
--
-- This is the core ingest subset: the tables the server writes and reads to
-- accept events, seal them into a tamper-evident hash chain, classify them,
-- and export evidence. A self-hoster points the server at any Postgres and
-- runs `migrate` once; nothing here depends on the hosted control plane.
--
-- Identifier types: organization, user, and Better Auth ids are text (they
-- come from an external identity system); everything the server mints is uuid.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Tenancy ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS projects (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    text NOT NULL,
    name               text NOT NULL,
    slug               text NOT NULL,
    description        text,
    created_by_user_id text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);

CREATE TABLE IF NOT EXISTS environments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id text NOT NULL,
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
-- A minimal Postgres-backed key store. In the hosted product these tables are
-- managed by the dashboard; the open server keeps the enforcement read. Key
-- issuance for a standalone server is a seed or CLI concern, not a UI.

CREATE TABLE IF NOT EXISTS apikey (
    id                     text PRIMARY KEY,
    config_id              text NOT NULL DEFAULT '',
    name                   text,
    start                  text,
    reference_id           text NOT NULL DEFAULT '',
    prefix                 text,
    key                    text NOT NULL,
    refill_interval        integer,
    refill_amount          integer,
    last_refill_at         timestamp,
    enabled                boolean DEFAULT true,
    rate_limit_enabled     boolean DEFAULT false,
    rate_limit_time_window integer,
    rate_limit_max         integer,
    request_count          integer,
    remaining              integer,
    last_request           timestamp,
    expires_at             timestamp,
    created_at             timestamp NOT NULL DEFAULT now(),
    updated_at             timestamp NOT NULL DEFAULT now(),
    permissions            text,
    metadata               text
);

CREATE TABLE IF NOT EXISTS api_key_metadata (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    better_auth_key_id text NOT NULL REFERENCES apikey (id) ON DELETE CASCADE,
    organization_id    text NOT NULL,
    project_id         uuid NOT NULL,
    environment_id     uuid NOT NULL,
    name               text NOT NULL,
    prefix             text,
    scopes             text[] NOT NULL DEFAULT '{}',
    created_by_user_id text NOT NULL,
    last_used_at       timestamptz,
    revoked_at         timestamptz,
    expires_at         timestamptz,
    ip_allowlist       jsonb,
    metadata           jsonb,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS api_key_metadata_better_auth_key_id_key
    ON api_key_metadata (better_auth_key_id);

-- Compliance catalog ----------------------------------------------------------

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
    organization_id   text NOT NULL,
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
    organization_id  text NOT NULL,
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
    id                  uuid PRIMARY KEY,
    organization_id     text NOT NULL,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    shard_index         integer NOT NULL,
    external_event_id   text,
    payload_hash        text NOT NULL,
    event_data          jsonb NOT NULL,
    classification      jsonb NOT NULL,
    control_mappings    jsonb NOT NULL,
    api_key_metadata_id uuid NOT NULL,
    idempotency_key     text,
    accepted_at         timestamptz NOT NULL DEFAULT now(),
    -- scope_check is last on purpose: it was appended by a later migration in
    -- the hosted schema, and the generated PendingEvent struct mirrors that
    -- order. Keep it last so column-list reads return the model verbatim.
    scope_check         jsonb
);

CREATE INDEX IF NOT EXISTS pending_events_shard
    ON pending_events (environment_id, shard_index, accepted_at, id);

-- Events (the immutable, hash-chained journal) --------------------------------

CREATE TABLE IF NOT EXISTS events (
    id                       uuid PRIMARY KEY,
    organization_id          text NOT NULL,
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
    organization_id text NOT NULL,
    event_id        uuid NOT NULL,
    data_classes    text[] NOT NULL DEFAULT '{}',
    risk_level      text,
    confidence      numeric,
    method          text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS event_control_mappings (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  text NOT NULL,
    event_id         uuid NOT NULL,
    framework        text NOT NULL,
    control_id       text NOT NULL,
    mapping_reason   text,
    confidence       numeric,
    mapped_by        text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    status           text NOT NULL DEFAULT 'auto',
    reviewer_user_id text,
    reviewed_at      timestamptz,
    reviewer_note    text
);

CREATE INDEX IF NOT EXISTS event_control_mappings_event ON event_control_mappings (organization_id, event_id);

-- Idempotency + rate limiting + usage -----------------------------------------

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     text NOT NULL,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    api_key_metadata_id uuid NOT NULL,
    key                 text NOT NULL,
    request_hash        text NOT NULL,
    response_body       bytea NOT NULL,
    status_code         integer,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL,
    UNIQUE (organization_id, key)
);

CREATE TABLE IF NOT EXISTS rate_limit_buckets (
    api_key_metadata_id uuid NOT NULL,
    bucket              text NOT NULL,
    window_start        timestamptz NOT NULL,
    count               integer NOT NULL DEFAULT 0,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (api_key_metadata_id, bucket, window_start)
);

CREATE TABLE IF NOT EXISTS org_usage_counters (
    organization_id text NOT NULL,
    period_start    timestamptz NOT NULL,
    accepted_count  bigint NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, period_start)
);

CREATE TABLE IF NOT EXISTS org_billing_state (
    organization_id        text PRIMARY KEY,
    overage_limit_cents    integer,
    overage_limit_disabled boolean NOT NULL DEFAULT false,
    reported_event_count   bigint NOT NULL DEFAULT 0,
    notified_threshold     integer NOT NULL DEFAULT 0,
    period_start           timestamptz,
    updated_at             timestamptz NOT NULL DEFAULT now()
);

-- Evidence + exceptions -------------------------------------------------------

CREATE TABLE IF NOT EXISTS evidence_exports (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     text NOT NULL,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    created_by_user_id  text NOT NULL,
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
    organization_id   text NOT NULL,
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    title             text NOT NULL,
    description       text,
    severity          text NOT NULL,
    status            text NOT NULL,
    owner_user_id     text,
    opened_at         timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz,
    resolved_at       timestamptz,
    related_event_ids uuid[],
    control_ids       text[],
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Registry + subscription (read-only for the open server) ---------------------
-- The server only reads these. Agent management and billing are hosted control
-- plane concerns; the tables exist so a standalone server's reads never error.

CREATE TABLE IF NOT EXISTS agents (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    text NOT NULL,
    project_id         uuid NOT NULL,
    name               text NOT NULL,
    description        text,
    model              text NOT NULL DEFAULT '',
    external_id        text NOT NULL,
    owner_user_id      text NOT NULL,
    supervisor_user_id text,
    authorized_scopes  text[] NOT NULL DEFAULT '{}',
    status             text NOT NULL DEFAULT 'active',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, project_id, external_id)
);

CREATE TABLE IF NOT EXISTS subscription (
    id                     text PRIMARY KEY,
    plan                   text NOT NULL,
    reference_id           text NOT NULL,
    stripe_customer_id     text,
    stripe_subscription_id text,
    status                 text NOT NULL,
    period_start           timestamp,
    period_end             timestamp,
    trial_start            timestamp,
    trial_end              timestamp,
    cancel_at_period_end   boolean,
    cancel_at              timestamp,
    canceled_at            timestamp,
    ended_at               timestamp,
    seats                  integer,
    billing_interval       text,
    stripe_schedule_id     text
);

CREATE INDEX IF NOT EXISTS subscription_reference_id ON subscription (reference_id);
