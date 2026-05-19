-- Add per-group request compatibility switch.
-- Disabled by default; when enabled, selected upstream request-format 400s may be retried internally.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS request_compat_enabled BOOLEAN NOT NULL DEFAULT FALSE;
