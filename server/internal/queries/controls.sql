-- name: ResolveControlReferences :many
-- Filters proposed mappings against the seeded controls catalog.
-- Pass a JSONB array: [{"framework":"SOC2","control_id":"CC6.1"}, ...]
SELECT c.framework, c.control_id
FROM controls c
JOIN jsonb_to_recordset($1::jsonb) AS proposed(framework text, control_id text)
  ON c.framework = proposed.framework AND c.control_id = proposed.control_id;

-- name: ListControlsForFramework :many
SELECT id, framework, control_id, title, description, category
FROM controls
WHERE framework = $1
ORDER BY control_id;
