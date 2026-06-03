-- Idempotent setup the server runs on boot: seed the baseline compliance
-- catalog, and (in no-auth mode) ensure a default project and environment.

-- name: UpsertFramework :exec
INSERT INTO frameworks (id, name, description, version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO NOTHING;

-- name: UpsertControl :exec
INSERT INTO controls (framework, control_id, title, description, category)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (framework, control_id) DO NOTHING;

-- name: EnsureDefaultProject :exec
INSERT INTO projects (id, organization_id, name, slug)
VALUES ($1, 'default', 'Default', 'default')
ON CONFLICT (id) DO NOTHING;

-- name: EnsureDefaultEnvironment :exec
INSERT INTO environments (id, organization_id, project_id, name, slug, type)
VALUES ($1, 'default', $2, 'Default', 'default', 'production')
ON CONFLICT (id) DO NOTHING;
