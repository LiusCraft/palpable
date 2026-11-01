-- 家长账号：微信登录与会话，见 docs/modules/account.md。
-- wx_openid：小程序 openid，空表示不是微信账号（演示家长）。
-- guardian_session：只存令牌的 SHA-256，库泄露也拿不到可用的令牌。
ALTER TABLE guardian ADD COLUMN wx_openid  TEXT    NOT NULL DEFAULT '';
ALTER TABLE guardian ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX guardian_wx_openid ON guardian (wx_openid) WHERE wx_openid != '';

CREATE TABLE guardian_session (
    token_hash  TEXT PRIMARY KEY,
    guardian_id TEXT    NOT NULL,
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL
);
CREATE INDEX guardian_session_guardian ON guardian_session (guardian_id);
