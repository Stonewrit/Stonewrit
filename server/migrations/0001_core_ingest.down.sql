-- Reverse of 0001_core_ingest. Drops the core ingest schema. Destructive:
-- every event, chain, and export is removed. Intended for development resets,
-- not production rollback.

DROP TABLE IF EXISTS subscription;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS exceptions;
DROP TABLE IF EXISTS evidence_exports;
DROP TABLE IF EXISTS org_billing_state;
DROP TABLE IF EXISTS org_usage_counters;
DROP TABLE IF EXISTS rate_limit_buckets;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS event_control_mappings;
DROP TABLE IF EXISTS event_classifications;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS pending_events;
DROP TABLE IF EXISTS chain_checkpoints;
DROP TABLE IF EXISTS chains;
DROP TABLE IF EXISTS controls;
DROP TABLE IF EXISTS frameworks;
DROP TABLE IF EXISTS api_key_metadata;
DROP TABLE IF EXISTS apikey;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS projects;
