-- name: GetEnvironmentForOrg :one
SELECT id, organization_id, project_id, slug, name, type, region
FROM environments
WHERE id = $1 AND organization_id = $2;

-- name: ListEnvironmentsForProject :many
SELECT id, slug, name, type, region
FROM environments
WHERE project_id = $1 AND organization_id = $2
ORDER BY created_at ASC;
