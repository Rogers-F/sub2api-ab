-- Add per-group signature compatibility switches.
-- NULL means inherit the existing global/account-level behavior.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS signature_compat_enabled BOOLEAN,
    ADD COLUMN IF NOT EXISTS signature_tool_text_downgrade_enabled BOOLEAN;
