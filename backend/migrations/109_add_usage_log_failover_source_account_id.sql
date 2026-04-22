-- 109_add_usage_log_failover_source_account_id.sql
-- Persist the source account that triggered account-level failover for admin usage logs.

ALTER TABLE usage_logs
ADD COLUMN IF NOT EXISTS failover_source_account_id BIGINT;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'usage_logs_failover_source_account_id_fkey'
  ) THEN
    ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_failover_source_account_id_fkey
    FOREIGN KEY (failover_source_account_id) REFERENCES accounts(id) ON DELETE SET NULL;
  END IF;
END $$;

COMMENT ON COLUMN usage_logs.failover_source_account_id IS '账号兜底时，记录发生错误并触发切换的来源账号 ID；无兜底则为空。';
