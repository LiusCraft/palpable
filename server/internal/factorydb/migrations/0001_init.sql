-- 厂家后台（palpable-admin）的库，独立于 palpabled 的库，见 docs/factory.md。
-- 这里没有任何儿童或家长的可识别信息：owner_ref 是 palpabled 做过 HMAC 的不可逆值。

CREATE TABLE batch (
    code       TEXT PRIMARY KEY,
    model      TEXT    NOT NULL,
    skin       TEXT    NOT NULL,
    firmware   TEXT    NOT NULL,
    note       TEXT    NOT NULL,
    created_by TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE unit (
    serial         TEXT PRIMARY KEY,
    mac            TEXT    NOT NULL UNIQUE,
    public_key     BLOB    NOT NULL,
    batch_code     TEXT    NOT NULL REFERENCES batch (code),
    skin           TEXT    NOT NULL,
    firmware       TEXT    NOT NULL,
    state          TEXT    NOT NULL,
    resume_state   TEXT    NOT NULL DEFAULT '',
    grade          TEXT    NOT NULL DEFAULT 'new',
    refurb_count   INTEGER NOT NULL DEFAULT 0,
    channel        TEXT    NOT NULL DEFAULT '',
    order_ref      TEXT    NOT NULL DEFAULT '',
    sold_at        INTEGER NOT NULL DEFAULT 0,
    warranty_until INTEGER NOT NULL DEFAULT 0,
    owner_ref      TEXT    NOT NULL DEFAULT '',
    prev_owner_ref TEXT    NOT NULL DEFAULT '',
    transfer_count INTEGER NOT NULL DEFAULT 0,
    locked         INTEGER NOT NULL DEFAULT 0,
    lock_reason    TEXT    NOT NULL DEFAULT '',
    version        INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX unit_state   ON unit (state);
CREATE INDEX unit_batch   ON unit (batch_code);
CREATE INDEX unit_channel ON unit (channel);

-- 只追加：除了"核对异常"的两列，任何字段都不许改，也不许删。
CREATE TABLE unit_event (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    serial      TEXT    NOT NULL REFERENCES unit (serial),
    at          INTEGER NOT NULL,
    actor       TEXT    NOT NULL,
    action      TEXT    NOT NULL,
    from_state  TEXT    NOT NULL,
    to_state    TEXT    NOT NULL,
    note        TEXT    NOT NULL,
    anomaly     INTEGER NOT NULL DEFAULT 0,
    resolved_by TEXT    NOT NULL DEFAULT '',
    resolved_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX unit_event_serial  ON unit_event (serial, id);
CREATE INDEX unit_event_anomaly ON unit_event (anomaly, resolved_at);

CREATE TRIGGER unit_event_no_delete BEFORE DELETE ON unit_event
BEGIN SELECT RAISE(ABORT, 'unit_event is append-only'); END;

CREATE TRIGGER unit_event_no_update BEFORE UPDATE OF serial, at, actor, action, from_state, to_state, note, anomaly ON unit_event
BEGIN SELECT RAISE(ABORT, 'unit_event is append-only'); END;

CREATE TABLE channel (
    code       TEXT PRIMARY KEY,
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE staff (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    role          TEXT    NOT NULL,
    password_hash TEXT    NOT NULL,
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

-- 只存令牌的 SHA-256。
CREATE TABLE staff_session (
    token_hash TEXT PRIMARY KEY,
    staff_id   INTEGER NOT NULL REFERENCES staff (id),
    expires_at INTEGER NOT NULL
);
CREATE INDEX staff_session_staff ON staff_session (staff_id);
