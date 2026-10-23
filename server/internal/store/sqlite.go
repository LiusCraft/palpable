package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/LiusCraft/palpable/server/internal/id"
)

//go:embed migrations/*.sql
var migrations embed.FS

type SQLite struct{ db *sql.DB }

var _ Store = (*SQLite)(nil)

// Open 打开（必要时创建）path 处的数据库并执行未应用的迁移。
func Open(ctx context.Context, path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite 同时只允许一个写者；单连接让写入排队，而不是撞上 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Devices() DeviceRepo         { return sqlDevices{s.db} }
func (s *SQLite) Styles() StyleRepo           { return sqlStyles{s.db} }
func (s *SQLite) Guardians() GuardianRepo     { return sqlGuardians{s.db} }
func (s *SQLite) Friendships() FriendshipRepo { return sqlFriendships{s.db} }
func (s *SQLite) Groups() GroupRepo           { return sqlGroups{s.db} }
func (s *SQLite) History() HistoryRepo        { return sqlHistory{s.db} }

// migrate 按文件名顺序执行 migrations/ 下的脚本，已执行到第几个记在 PRAGMA user_version。
func migrate(ctx context.Context, db *sql.DB) error {
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version > len(names) {
		return fmt.Errorf("database version %d is newer than this build (%d)", version, len(names))
	}
	for i := version; i < len(names); i++ {
		script, err := migrations.ReadFile(names[i])
		if err != nil {
			return err
		}
		if err := inTx(ctx, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(script)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1))
			return err
		}); err != nil {
			return fmt.Errorf("apply %s: %w", names[i], err)
		}
	}
	return nil
}

func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func sqliteCode(err error) int {
	var e *sqlite.Error
	if errors.As(err, &e) {
		return e.Code()
	}
	return 0
}

func isUniqueViolation(err error) bool {
	c := sqliteCode(err)
	return c == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY || c == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func notFoundIfNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func expectOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func queryList[T any](ctx context.Context, db *sql.DB, scan func(rowScanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- device ----

type sqlDevices struct{ db *sql.DB }

const deviceCols = "device_id, skin, style_id, agent_id, doll_name, kid_name, stage, guardian_id, voice_consent, speaker_id, created_at"

func scanDevice(r rowScanner) (Device, error) {
	var d Device
	var created int64
	err := r.Scan(&d.ID, &d.Skin, &d.StyleID, &d.AgentID, &d.DollName, &d.KidName, &d.Stage, &d.Guardian, &d.VoiceConsent, &d.SpeakerID, &created)
	d.CreatedAt = fromMillis(created)
	return d, err
}

func (r sqlDevices) Get(ctx context.Context, d id.DeviceID) (Device, error) {
	dev, err := scanDevice(r.db.QueryRowContext(ctx, "SELECT "+deviceCols+" FROM device WHERE device_id = ?", d))
	if err != nil {
		return Device{}, fmt.Errorf("get device %s: %w", d, notFoundIfNoRows(err))
	}
	return dev, nil
}

func (r sqlDevices) Create(ctx context.Context, d Device) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO device ("+deviceCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		d.ID, d.Skin, d.StyleID, d.AgentID, d.DollName, d.KidName, d.Stage, d.Guardian, d.VoiceConsent, d.SpeakerID, toMillis(d.CreatedAt))
	if isUniqueViolation(err) {
		err = ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create device %s: %w", d.ID, err)
	}
	return nil
}

func (r sqlDevices) Update(ctx context.Context, d Device) error {
	res, err := r.db.ExecContext(ctx, `UPDATE device SET skin = ?, style_id = ?, agent_id = ?, doll_name = ?, kid_name = ?,
		stage = ?, guardian_id = ?, voice_consent = ?, speaker_id = ? WHERE device_id = ?`,
		d.Skin, d.StyleID, d.AgentID, d.DollName, d.KidName, d.Stage, d.Guardian, d.VoiceConsent, d.SpeakerID, d.ID)
	if err == nil {
		err = expectOneRow(res)
	}
	if err != nil {
		return fmt.Errorf("update device %s: %w", d.ID, err)
	}
	return nil
}

func (r sqlDevices) Delete(ctx context.Context, d id.DeviceID) error {
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM device WHERE device_id = ?", d)
		if err != nil {
			return err
		}
		if err := expectOneRow(res); err != nil {
			return err
		}
		for _, q := range []struct {
			sql string
			arg any
		}{
			{"DELETE FROM friendship WHERE a = ?1 OR b = ?1", d},
			{"DELETE FROM channel_member WHERE participant_id = ?", id.Device(d)},
			{"DELETE FROM history_event WHERE device_id = ?", d},
		} {
			if _, err := tx.ExecContext(ctx, q.sql, q.arg); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete device %s: %w", d, err)
	}
	return nil
}

// ---- style ----

type sqlStyles struct{ db *sql.DB }

func scanStyle(r rowScanner) (Style, error) {
	var s Style
	err := r.Scan(&s.ID, &s.Skin, &s.AgentID)
	return s, err
}

func (r sqlStyles) Get(ctx context.Context, styleID string) (Style, error) {
	s, err := scanStyle(r.db.QueryRowContext(ctx, "SELECT style_id, skin, agent_id FROM style WHERE style_id = ?", styleID))
	if err != nil {
		return Style{}, fmt.Errorf("get style %s: %w", styleID, notFoundIfNoRows(err))
	}
	return s, nil
}

func (r sqlStyles) Put(ctx context.Context, s Style) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO style (style_id, skin, agent_id) VALUES (?, ?, ?)
		ON CONFLICT (style_id) DO UPDATE SET skin = excluded.skin, agent_id = excluded.agent_id`, s.ID, s.Skin, s.AgentID)
	if err != nil {
		return fmt.Errorf("put style %s: %w", s.ID, err)
	}
	return nil
}

func (r sqlStyles) List(ctx context.Context) ([]Style, error) {
	out, err := queryList(ctx, r.db, scanStyle, "SELECT style_id, skin, agent_id FROM style ORDER BY style_id")
	if err != nil {
		return nil, fmt.Errorf("list styles: %w", err)
	}
	return out, nil
}

// ---- guardian ----

type sqlGuardians struct{ db *sql.DB }

func (r sqlGuardians) Get(ctx context.Context, g id.GuardianID) (Guardian, error) {
	out := Guardian{ID: g}
	err := r.db.QueryRowContext(ctx, "SELECT display_name FROM guardian WHERE guardian_id = ?", g).Scan(&out.DisplayName)
	if err != nil {
		return Guardian{}, fmt.Errorf("get guardian %s: %w", g, notFoundIfNoRows(err))
	}
	return out, nil
}

func (r sqlGuardians) Put(ctx context.Context, g Guardian) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO guardian (guardian_id, display_name) VALUES (?, ?)
		ON CONFLICT (guardian_id) DO UPDATE SET display_name = excluded.display_name`, g.ID, g.DisplayName)
	if err != nil {
		return fmt.Errorf("put guardian %s: %w", g.ID, err)
	}
	return nil
}

// ---- friendship ----

type sqlFriendships struct{ db *sql.DB }

const friendshipCols = "id, a, b, status, confirmed_by, created_at"

func scanFriendship(r rowScanner) (Friendship, error) {
	var f Friendship
	var created int64
	err := r.Scan(&f.ID, &f.A, &f.B, &f.Status, &f.ConfirmedBy, &created)
	f.CreatedAt = fromMillis(created)
	return f, err
}

func (r sqlFriendships) Get(ctx context.Context, friendshipID string) (Friendship, error) {
	f, err := scanFriendship(r.db.QueryRowContext(ctx, "SELECT "+friendshipCols+" FROM friendship WHERE id = ?", friendshipID))
	if err != nil {
		return Friendship{}, fmt.Errorf("get friendship %s: %w", friendshipID, notFoundIfNoRows(err))
	}
	return f, nil
}

func (r sqlFriendships) ByPair(ctx context.Context, a, b id.DeviceID) (Friendship, error) {
	a, b = orderPair(a, b)
	f, err := scanFriendship(r.db.QueryRowContext(ctx, "SELECT "+friendshipCols+" FROM friendship WHERE a = ? AND b = ?", a, b))
	if err != nil {
		return Friendship{}, fmt.Errorf("get friendship %s/%s: %w", a, b, notFoundIfNoRows(err))
	}
	return f, nil
}

func (r sqlFriendships) Create(ctx context.Context, f Friendship) error {
	if f.A == f.B {
		return fmt.Errorf("create friendship %s: %w", f.ID, errSelfFriendship)
	}
	f.A, f.B = orderPair(f.A, f.B)
	_, err := r.db.ExecContext(ctx, "INSERT INTO friendship ("+friendshipCols+") VALUES (?, ?, ?, ?, ?, ?)",
		f.ID, f.A, f.B, f.Status, f.ConfirmedBy, toMillis(f.CreatedAt))
	if isUniqueViolation(err) {
		err = ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create friendship %s: %w", f.ID, err)
	}
	return nil
}

func (r sqlFriendships) Update(ctx context.Context, f Friendship) error {
	res, err := r.db.ExecContext(ctx, "UPDATE friendship SET status = ?, confirmed_by = ? WHERE id = ?", f.Status, f.ConfirmedBy, f.ID)
	if err == nil {
		err = expectOneRow(res)
	}
	if err != nil {
		return fmt.Errorf("update friendship %s: %w", f.ID, err)
	}
	return nil
}

func (r sqlFriendships) ListByDevice(ctx context.Context, d id.DeviceID) ([]Friendship, error) {
	out, err := queryList(ctx, r.db, scanFriendship,
		"SELECT "+friendshipCols+" FROM friendship WHERE a = ?1 OR b = ?1 ORDER BY created_at, id", d)
	if err != nil {
		return nil, fmt.Errorf("list friendships of %s: %w", d, err)
	}
	return out, nil
}

func (r sqlFriendships) ListByStatus(ctx context.Context, status int) ([]Friendship, error) {
	out, err := queryList(ctx, r.db, scanFriendship,
		"SELECT "+friendshipCols+" FROM friendship WHERE status = ? ORDER BY created_at, id", status)
	if err != nil {
		return nil, fmt.Errorf("list friendships with status %d: %w", status, err)
	}
	return out, nil
}

// ---- group ----

type sqlGroups struct{ db *sql.DB }

const groupCols = "channel_id, name, owner, code, expires_at"

func scanGroup(r rowScanner) (Group, error) {
	var g Group
	var expires int64
	err := r.Scan(&g.ID, &g.Name, &g.Owner, &g.Code, &expires)
	g.ExpiresAt = fromMillis(expires)
	return g, err
}

func scanMember(r rowScanner) (Member, error) {
	var m Member
	var joined int64
	err := r.Scan(&m.Participant, &joined)
	m.JoinedAt = fromMillis(joined)
	return m, err
}

func (r sqlGroups) Create(ctx context.Context, g Group) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO channel_group ("+groupCols+") VALUES (?, ?, ?, ?, ?)",
		g.ID, g.Name, g.Owner, g.Code, toMillis(g.ExpiresAt))
	if isUniqueViolation(err) {
		err = ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create group %s: %w", g.ID, err)
	}
	return nil
}

func (r sqlGroups) Get(ctx context.Context, ch id.ChannelID) (Group, error) {
	g, err := scanGroup(r.db.QueryRowContext(ctx, "SELECT "+groupCols+" FROM channel_group WHERE channel_id = ?", ch))
	if err != nil {
		return Group{}, fmt.Errorf("get group %s: %w", ch, notFoundIfNoRows(err))
	}
	return g, nil
}

func (r sqlGroups) ByCode(ctx context.Context, code string, now time.Time) (Group, error) {
	g, err := scanGroup(r.db.QueryRowContext(ctx, "SELECT "+groupCols+` FROM channel_group
		WHERE code = ? AND (expires_at = 0 OR expires_at > ?) ORDER BY channel_id LIMIT 1`, code, toMillis(now)))
	if err != nil {
		return Group{}, fmt.Errorf("get group by code: %w", notFoundIfNoRows(err))
	}
	return g, nil
}

func (r sqlGroups) List(ctx context.Context) ([]Group, error) {
	out, err := queryList(ctx, r.db, scanGroup, "SELECT "+groupCols+" FROM channel_group ORDER BY channel_id")
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return out, nil
}

func (r sqlGroups) Delete(ctx context.Context, ch id.ChannelID) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM channel_group WHERE channel_id = ?", ch)
	if err == nil {
		err = expectOneRow(res)
	}
	if err != nil {
		return fmt.Errorf("delete group %s: %w", ch, err)
	}
	return nil
}

func (r sqlGroups) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, "DELETE FROM channel_group WHERE expires_at != 0 AND expires_at <= ?", toMillis(now))
	if err != nil {
		return 0, fmt.Errorf("delete expired groups: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired groups: %w", err)
	}
	return int(n), nil
}

func (r sqlGroups) AddMember(ctx context.Context, ch id.ChannelID, m Member) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO channel_member (channel_id, participant_id, joined_at) VALUES (?, ?, ?)
		ON CONFLICT (channel_id, participant_id) DO NOTHING`, ch, m.Participant, toMillis(m.JoinedAt))
	if sqliteCode(err) == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		err = ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("add %s to group %s: %w", m.Participant, ch, err)
	}
	return nil
}

func (r sqlGroups) RemoveMember(ctx context.Context, ch id.ChannelID, p id.ParticipantID) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM channel_member WHERE channel_id = ? AND participant_id = ?", ch, p); err != nil {
		return fmt.Errorf("remove %s from group %s: %w", p, ch, err)
	}
	return nil
}

func (r sqlGroups) Members(ctx context.Context, ch id.ChannelID) ([]Member, error) {
	out, err := queryList(ctx, r.db, scanMember,
		"SELECT participant_id, joined_at FROM channel_member WHERE channel_id = ? ORDER BY joined_at, participant_id", ch)
	if err != nil {
		return nil, fmt.Errorf("list members of %s: %w", ch, err)
	}
	return out, nil
}

func (r sqlGroups) ListByMember(ctx context.Context, p id.ParticipantID) ([]Group, error) {
	out, err := queryList(ctx, r.db, scanGroup, `SELECT g.channel_id, g.name, g.owner, g.code, g.expires_at
		FROM channel_group g JOIN channel_member m ON m.channel_id = g.channel_id
		WHERE m.participant_id = ? ORDER BY m.joined_at, g.channel_id`, p)
	if err != nil {
		return nil, fmt.Errorf("list groups of %s: %w", p, err)
	}
	return out, nil
}

// ---- history ----

type sqlHistory struct{ db *sql.DB }

func scanEvent(r rowScanner) (HistoryEvent, error) {
	var e HistoryEvent
	var payload string
	var at int64
	err := r.Scan(&e.ID, &e.Device, &e.Kind, &payload, &at)
	e.Payload = []byte(payload)
	e.At = fromMillis(at)
	return e, err
}

func (r sqlHistory) Append(ctx context.Context, e HistoryEvent) error {
	payload := string(e.Payload)
	if payload == "" {
		payload = "{}"
	}
	_, err := r.db.ExecContext(ctx, "INSERT INTO history_event (id, device_id, kind, payload, at) VALUES (?, ?, ?, ?, ?)",
		e.ID, e.Device, e.Kind, payload, toMillis(e.At))
	if isUniqueViolation(err) {
		err = ErrConflict
	}
	if err != nil {
		return fmt.Errorf("append history %s: %w", e.ID, err)
	}
	return nil
}

func (r sqlHistory) ListByDevice(ctx context.Context, d id.DeviceID, from, to time.Time) ([]HistoryEvent, error) {
	out, err := queryList(ctx, r.db, scanEvent, `SELECT id, device_id, kind, payload, at FROM history_event
		WHERE device_id = ? AND at >= ? AND at < ? ORDER BY at, id`, d, toMillis(from), toMillis(to))
	if err != nil {
		return nil, fmt.Errorf("list history of %s: %w", d, err)
	}
	return out, nil
}
