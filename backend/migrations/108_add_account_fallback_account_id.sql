-- 108_add_account_fallback_account_id.sql
-- Add account-level chained fallback support.

ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS fallback_account_id BIGINT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'accounts_fallback_account_id_fkey'
  ) THEN
    ALTER TABLE accounts
    ADD CONSTRAINT accounts_fallback_account_id_fkey
    FOREIGN KEY (fallback_account_id) REFERENCES accounts(id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_accounts_fallback_account_id
ON accounts(fallback_account_id) WHERE deleted_at IS NULL;

COMMENT ON COLUMN accounts.fallback_account_id IS '账号兜底链路中的下一跳账号 ID；仅允许同平台账号，删除目标账号时自动置空。';
