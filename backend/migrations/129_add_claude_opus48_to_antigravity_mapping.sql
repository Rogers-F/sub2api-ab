-- Add Claude Opus 4.8 passthrough to persisted Antigravity model mappings.
--
-- Existing Antigravity accounts can carry a stored credentials.model_mapping
-- from earlier migrations. Without an exact 4.8 entry, those accounts either
-- reject claude-opus-4-8 during scheduling or allow a stale wildcard to route it
-- back to an older Opus target.

UPDATE accounts
SET credentials = jsonb_set(
    COALESCE(credentials, '{}'::jsonb),
    '{model_mapping,claude-opus-4-8}',
    '"claude-opus-4-8"'::jsonb,
    true
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
