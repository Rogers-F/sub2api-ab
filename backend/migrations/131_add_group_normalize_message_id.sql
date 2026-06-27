-- Add per-group Claude Messages response id normalization switch.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS normalize_message_id_enabled BOOLEAN NOT NULL DEFAULT FALSE;
