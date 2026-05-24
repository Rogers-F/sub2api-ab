-- Add per-group smart dispatch configuration.
-- When enabled, a target group can move normal accounts from a configured pool group.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS smart_dispatch_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS smart_dispatch_source_group_id BIGINT,
    ADD COLUMN IF NOT EXISTS smart_dispatch_count INTEGER NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_smart_dispatch_source_group_id_fkey'
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_smart_dispatch_source_group_id_fkey
            FOREIGN KEY (smart_dispatch_source_group_id)
            REFERENCES groups(id)
            ON DELETE SET NULL;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_groups_smart_dispatch_source_group_id
    ON groups(smart_dispatch_source_group_id)
    WHERE smart_dispatch_source_group_id IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_smart_dispatch_count_positive'
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_smart_dispatch_count_positive
            CHECK (smart_dispatch_count > 0);
    END IF;
END
$$;
