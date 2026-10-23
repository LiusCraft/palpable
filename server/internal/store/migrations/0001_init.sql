-- 时间统一存 Unix 毫秒（UTC），0 表示无。

CREATE TABLE device (
    device_id     TEXT PRIMARY KEY,
    skin          TEXT    NOT NULL DEFAULT '',
    style_id      TEXT    NOT NULL DEFAULT '',
    agent_id      TEXT    NOT NULL DEFAULT '',
    doll_name     TEXT    NOT NULL DEFAULT '',
    kid_name      TEXT    NOT NULL DEFAULT '',
    stage         INTEGER NOT NULL DEFAULT 0,
    guardian_id   TEXT    NOT NULL DEFAULT '',
    voice_consent INTEGER NOT NULL DEFAULT 0,
    speaker_id    TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL
);

CREATE TABLE style (
    style_id TEXT PRIMARY KEY,
    skin     TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE guardian (
    guardian_id  TEXT PRIMARY KEY,
    display_name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE friendship (
    id           TEXT PRIMARY KEY,
    a            TEXT    NOT NULL,
    b            TEXT    NOT NULL,
    status       INTEGER NOT NULL,
    confirmed_by TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    CHECK (a < b),
    UNIQUE (a, b)
);
CREATE INDEX friendship_b ON friendship (b);

CREATE TABLE channel_group (
    channel_id TEXT PRIMARY KEY,
    name       TEXT    NOT NULL DEFAULT '',
    owner      TEXT    NOT NULL,
    code       TEXT    NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX channel_group_code ON channel_group (code);

CREATE TABLE channel_member (
    channel_id     TEXT    NOT NULL REFERENCES channel_group (channel_id) ON DELETE CASCADE,
    participant_id TEXT    NOT NULL,
    joined_at      INTEGER NOT NULL,
    PRIMARY KEY (channel_id, participant_id)
);
CREATE INDEX channel_member_participant ON channel_member (participant_id);

CREATE TABLE history_event (
    id        TEXT PRIMARY KEY,
    device_id TEXT    NOT NULL,
    kind      INTEGER NOT NULL,
    payload   TEXT    NOT NULL DEFAULT '{}',
    at        INTEGER NOT NULL
);
CREATE INDEX history_event_device_at ON history_event (device_id, at);
