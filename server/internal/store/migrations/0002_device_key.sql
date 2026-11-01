-- 设备公钥：认证用，只存公钥（SPKI DER）。与 device 档案分开：解绑删档案时公钥保留，设备仍是同一台。
CREATE TABLE device_key (
    device_id  TEXT PRIMARY KEY,
    public_key BLOB    NOT NULL,
    created_at INTEGER NOT NULL
);
