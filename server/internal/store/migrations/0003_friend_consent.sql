-- 交友同意分两侧：每一侧由自家监护人放行（或该家长设为"孩子自己交友"时自动放行），见 docs/trust.md §4。
ALTER TABLE friendship ADD COLUMN approved_a TEXT    NOT NULL DEFAULT '';
ALTER TABLE friendship ADD COLUMN approved_b TEXT    NOT NULL DEFAULT '';
ALTER TABLE friendship ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
-- 家长为自家孩子设置的交友方式：0 需我批准（默认），1 孩子自己交友、事后通知。
ALTER TABLE device ADD COLUMN friend_policy INTEGER NOT NULL DEFAULT 0;
