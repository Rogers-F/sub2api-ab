-- Add smart dispatch target normal-account floor.
-- Default 1 preserves the original behavior: refill only when the target group has no normal accounts.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS smart_dispatch_min_normal_accounts INTEGER NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_smart_dispatch_min_normal_accounts_positive'
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_smart_dispatch_min_normal_accounts_positive
            CHECK (smart_dispatch_min_normal_accounts > 0);
    END IF;
END
$$;
