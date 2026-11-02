// Package factorydb 是厂家后台 factory.Repo 与 factory.StaffRepo 的 SQLite 实现。
package factorydb

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/LiusCraft/palpable/server/internal/factory"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct{ db *sql.DB }

var (
	_ factory.Repo      = (*DB)(nil)
	_ factory.StaffRepo = (*DB)(nil)
)

// Open 打开（必要时创建）path 处的库并执行未应用的迁移。
func Open(ctx context.Context, path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

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

func isUnique(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	c := e.Code()
	return c == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY || c == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return factory.ErrNotFound
	}
	return err
}

func ts(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromTS(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

type scanner interface{ Scan(dest ...any) error }

const unitCols = `serial, mac, public_key, batch_code, skin, firmware, state, resume_state, grade, refurb_count,
channel, order_ref, sold_at, warranty_until, owner_ref, prev_owner_ref, transfer_count, locked, lock_reason,
version, created_at, updated_at`

func scanUnit(r scanner) (factory.Unit, error) {
	var u factory.Unit
	var state, resume, grade, reason string
	var sold, warranty, created, updated int64
	var locked int
	err := r.Scan(&u.Serial, &u.MAC, &u.PublicKey, &u.BatchCode, &u.Skin, &u.Firmware, &state, &resume, &grade,
		&u.RefurbCount, &u.Channel, &u.OrderRef, &sold, &warranty, &u.OwnerRef, &u.PrevOwnerRef, &u.TransferCount,
		&locked, &reason, &u.Version, &created, &updated)
	if err != nil {
		return factory.Unit{}, err
	}
	u.State, u.ResumeState, u.Grade, u.LockReason = factory.State(state), factory.State(resume), factory.Grade(grade), factory.LockReason(reason)
	u.SoldAt, u.WarrantyUntil, u.CreatedAt, u.UpdatedAt = fromTS(sold), fromTS(warranty), fromTS(created), fromTS(updated)
	u.Locked = locked != 0
	return u, nil
}

func insertUnit(ctx context.Context, tx *sql.Tx, u factory.Unit) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO unit (`+unitCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.Serial, u.MAC, u.PublicKey, u.BatchCode, u.Skin, u.Firmware, string(u.State), string(u.ResumeState), string(u.Grade),
		u.RefurbCount, u.Channel, u.OrderRef, ts(u.SoldAt), ts(u.WarrantyUntil), u.OwnerRef, u.PrevOwnerRef,
		u.TransferCount, b2i(u.Locked), string(u.LockReason), u.Version, ts(u.CreatedAt), ts(u.UpdatedAt))
	return err
}

func insertEvent(ctx context.Context, tx *sql.Tx, e factory.Event) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO unit_event (serial, at, actor, action, from_state, to_state, note, anomaly)
VALUES (?,?,?,?,?,?,?,?)`, e.Serial, ts(e.At), e.Actor, e.Action, string(e.From), string(e.To), e.Note, b2i(e.Anomaly))
	return err
}

func (d *DB) CreateBatch(ctx context.Context, b factory.Batch, units []factory.Unit, events []factory.Event) error {
	err := inTx(ctx, d.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO batch (code, model, skin, firmware, note, created_by, created_at) VALUES (?,?,?,?,?,?,?)`,
			b.Code, b.Model, b.Skin, b.Firmware, b.Note, b.CreatedBy, ts(b.CreatedAt)); err != nil {
			return err
		}
		for _, u := range units {
			if err := insertUnit(ctx, tx, u); err != nil {
				return err
			}
		}
		for _, e := range events {
			if err := insertEvent(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
	if isUnique(err) {
		return fmt.Errorf("batch %s: %w", b.Code, factory.ErrConflict)
	}
	return err
}

func (d *DB) Batches(ctx context.Context) ([]factory.BatchSummary, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT b.code, b.model, b.skin, b.firmware, b.note, b.created_by, b.created_at, COUNT(u.serial)
FROM batch b LEFT JOIN unit u ON u.batch_code = b.code GROUP BY b.code ORDER BY b.created_at DESC, b.code DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []factory.BatchSummary
	for rows.Next() {
		var s factory.BatchSummary
		var at int64
		if err := rows.Scan(&s.Code, &s.Model, &s.Skin, &s.Firmware, &s.Note, &s.CreatedBy, &at, &s.Units); err != nil {
			return nil, err
		}
		s.CreatedAt = fromTS(at)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) Unit(ctx context.Context, serial string) (factory.Unit, error) {
	u, err := scanUnit(d.db.QueryRowContext(ctx, `SELECT `+unitCols+` FROM unit WHERE serial = ?`, serial))
	return u, notFound(err)
}

func (d *DB) UnitByMAC(ctx context.Context, mac string) (factory.Unit, error) {
	u, err := scanUnit(d.db.QueryRowContext(ctx, `SELECT `+unitCols+` FROM unit WHERE mac = ?`, mac))
	return u, notFound(err)
}

func (d *DB) SaveUnit(ctx context.Context, u factory.Unit, events ...factory.Event) error {
	return inTx(ctx, d.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE unit SET state=?, resume_state=?, grade=?, refurb_count=?, channel=?, order_ref=?,
sold_at=?, warranty_until=?, owner_ref=?, prev_owner_ref=?, transfer_count=?, locked=?, lock_reason=?, updated_at=?, version=version+1
WHERE serial=? AND version=?`,
			string(u.State), string(u.ResumeState), string(u.Grade), u.RefurbCount, u.Channel, u.OrderRef, ts(u.SoldAt),
			ts(u.WarrantyUntil), u.OwnerRef, u.PrevOwnerRef, u.TransferCount, b2i(u.Locked), string(u.LockReason),
			ts(u.UpdatedAt), u.Serial, u.Version)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var one int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM unit WHERE serial=?`, u.Serial).Scan(&one); err != nil {
				return notFound(err)
			}
			return fmt.Errorf("unit %s changed since it was read: %w", u.Serial, factory.ErrConflict)
		}
		for _, e := range events {
			if err := insertEvent(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DB) ListUnits(ctx context.Context, f factory.Filter) ([]factory.Unit, int, error) {
	var where []string
	var args []any
	if f.State != "" {
		where, args = append(where, "state = ?"), append(args, string(f.State))
	}
	if f.Channel != "" {
		where, args = append(where, "channel = ?"), append(args, f.Channel)
	}
	if f.Batch != "" {
		where, args = append(where, "batch_code = ?"), append(args, f.Batch)
	}
	if f.Locked != nil {
		where, args = append(where, "locked = ?"), append(args, b2i(*f.Locked))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + escapeLike(strings.ToLower(q)) + "%"
		where = append(where, `(LOWER(serial) LIKE ? ESCAPE '\' OR mac LIKE ? ESCAPE '\' OR LOWER(order_ref) LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT `+unitCols+` FROM unit`+clause+` ORDER BY updated_at DESC, serial LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []factory.Unit
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

const eventCols = `id, serial, at, actor, action, from_state, to_state, note, anomaly, resolved_by, resolved_at`

func scanEvent(r scanner) (factory.Event, error) {
	var e factory.Event
	var at, resolved int64
	var from, to string
	var anomaly int
	if err := r.Scan(&e.ID, &e.Serial, &at, &e.Actor, &e.Action, &from, &to, &e.Note, &anomaly, &e.ResolvedBy, &resolved); err != nil {
		return factory.Event{}, err
	}
	e.At, e.ResolvedAt, e.From, e.To, e.Anomaly = fromTS(at), fromTS(resolved), factory.State(from), factory.State(to), anomaly != 0
	return e, nil
}

func (d *DB) events(ctx context.Context, query string, args ...any) ([]factory.Event, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []factory.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) Events(ctx context.Context, serial string) ([]factory.Event, error) {
	return d.events(ctx, `SELECT `+eventCols+` FROM unit_event WHERE serial = ? ORDER BY id`, serial)
}

func (d *DB) RecentEvents(ctx context.Context, f factory.EventFilter) ([]factory.Event, error) {
	var where []string
	var args []any
	if f.Serial != "" {
		where, args = append(where, "serial = ?"), append(args, f.Serial)
	}
	if f.OpenAnomaly {
		where = append(where, "anomaly = 1 AND resolved_at = 0")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	return d.events(ctx, `SELECT `+eventCols+` FROM unit_event`+clause+` ORDER BY id DESC LIMIT ?`, append(args, f.Limit)...)
}

func (d *DB) ResolveEvent(ctx context.Context, id int64, by string, at time.Time) error {
	res, err := d.db.ExecContext(ctx, `UPDATE unit_event SET resolved_by=?, resolved_at=? WHERE id=? AND anomaly=1 AND resolved_at=0`, by, ts(at), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return factory.ErrNotFound
	}
	return nil
}

func (d *DB) Stats(ctx context.Context) (factory.Stats, error) {
	st := factory.Stats{ByState: map[factory.State]int{}}
	rows, err := d.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM unit GROUP BY state`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return st, err
		}
		st.ByState[factory.State(s)] = n
		st.Total += n
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit WHERE locked = 1`).Scan(&st.Locked); err != nil {
		return st, err
	}
	err = d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM unit_event WHERE anomaly = 1 AND resolved_at = 0`).Scan(&st.OpenAnomalies)
	return st, err
}

func (d *DB) Channels(ctx context.Context) ([]factory.Channel, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT code, name, kind, created_at FROM channel ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []factory.Channel
	for rows.Next() {
		var c factory.Channel
		var kind string
		var at int64
		if err := rows.Scan(&c.Code, &c.Name, &kind, &at); err != nil {
			return nil, err
		}
		c.Kind, c.CreatedAt = factory.ChannelKind(kind), fromTS(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) Channel(ctx context.Context, code string) (factory.Channel, error) {
	var c factory.Channel
	var kind string
	var at int64
	err := d.db.QueryRowContext(ctx, `SELECT code, name, kind, created_at FROM channel WHERE code = ?`, code).Scan(&c.Code, &c.Name, &kind, &at)
	if err != nil {
		return factory.Channel{}, notFound(err)
	}
	c.Kind, c.CreatedAt = factory.ChannelKind(kind), fromTS(at)
	return c, nil
}

func (d *DB) PutChannel(ctx context.Context, c factory.Channel) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO channel (code, name, kind, created_at) VALUES (?,?,?,?)
ON CONFLICT(code) DO UPDATE SET name=excluded.name, kind=excluded.kind`, c.Code, c.Name, string(c.Kind), ts(c.CreatedAt))
	return err
}

// ---- 人员与会话 ----

const staffCols = `id, username, role, password_hash, disabled, created_at`

func scanStaff(r scanner) (factory.Staff, error) {
	var s factory.Staff
	var role string
	var disabled int
	var at int64
	if err := r.Scan(&s.ID, &s.Username, &role, &s.PasswordHash, &disabled, &at); err != nil {
		return factory.Staff{}, err
	}
	s.Role, s.Disabled, s.CreatedAt = factory.Role(role), disabled != 0, fromTS(at)
	return s, nil
}

func (d *DB) CreateStaff(ctx context.Context, s factory.Staff) (factory.Staff, error) {
	res, err := d.db.ExecContext(ctx, `INSERT INTO staff (username, role, password_hash, disabled, created_at) VALUES (?,?,?,?,?)`,
		s.Username, string(s.Role), s.PasswordHash, b2i(s.Disabled), ts(s.CreatedAt))
	if isUnique(err) {
		return factory.Staff{}, fmt.Errorf("username %s: %w", s.Username, factory.ErrConflict)
	}
	if err != nil {
		return factory.Staff{}, err
	}
	s.ID, err = res.LastInsertId()
	return s, err
}

func (d *DB) StaffByName(ctx context.Context, username string) (factory.Staff, error) {
	s, err := scanStaff(d.db.QueryRowContext(ctx, `SELECT `+staffCols+` FROM staff WHERE username = ?`, username))
	return s, notFound(err)
}

func (d *DB) StaffByID(ctx context.Context, id int64) (factory.Staff, error) {
	s, err := scanStaff(d.db.QueryRowContext(ctx, `SELECT `+staffCols+` FROM staff WHERE id = ?`, id))
	return s, notFound(err)
}

func (d *DB) Staffs(ctx context.Context) ([]factory.Staff, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+staffCols+` FROM staff ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []factory.Staff
	for rows.Next() {
		s, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) UpdateStaff(ctx context.Context, s factory.Staff) error {
	res, err := d.db.ExecContext(ctx, `UPDATE staff SET role=?, password_hash=?, disabled=? WHERE id=?`,
		string(s.Role), s.PasswordHash, b2i(s.Disabled), s.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return factory.ErrNotFound
	}
	return nil
}

func (d *DB) CountStaff(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM staff`).Scan(&n)
	return n, err
}

func (d *DB) PutSession(ctx context.Context, tokenHash string, staffID int64, expires time.Time) error {
	// 顺手清掉过期的会话，免得表一直长。
	if _, err := d.db.ExecContext(ctx, `DELETE FROM staff_session WHERE expires_at <= ?`, ts(time.Now())); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, `INSERT INTO staff_session (token_hash, staff_id, expires_at) VALUES (?,?,?)`, tokenHash, staffID, ts(expires))
	return err
}

func (d *DB) SessionStaff(ctx context.Context, tokenHash string, now time.Time) (factory.Staff, error) {
	s, err := scanStaff(d.db.QueryRowContext(ctx, `SELECT st.id, st.username, st.role, st.password_hash, st.disabled, st.created_at
FROM staff_session ss JOIN staff st ON st.id = ss.staff_id WHERE ss.token_hash = ? AND ss.expires_at > ?`, tokenHash, ts(now)))
	return s, notFound(err)
}

func (d *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM staff_session WHERE token_hash = ?`, tokenHash)
	return err
}

func (d *DB) DeleteSessionsOf(ctx context.Context, staffID int64) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM staff_session WHERE staff_id = ?`, staffID)
	return err
}
