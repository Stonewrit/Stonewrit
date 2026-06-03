-- Resource management used by the `stonewrit` CLI.

-- name: CreateProject :one
INSERT INTO projects (organization_id, name, slug)
VALUES ($1, $2, $3)
RETURNING id, organization_id, name, slug, created_at;

-- name: ListProjects :many
SELECT id, organization_id, name, slug, created_at
FROM projects
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateEnvironment :one
INSERT INTO environments (organization_id, project_id, name, slug, type)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, organization_id, project_id, name, slug, type, created_at;

-- name: ListEnvironments :many
SELECT id, organization_id, project_id, name, slug, type, created_at
FROM environments
WHERE deleted_at IS NULL
ORDER BY created_at DESC;
