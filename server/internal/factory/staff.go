package factory

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrBadLogin    = errors.New("factory: wrong username or password")
	ErrTooManyTry  = errors.New("factory: too many failed logins")
	ErrUnauth      = errors.New("factory: not logged in")
	usernameRE     = regexp.MustCompile(`^[a-z0-9._\-]{3,32}$`)
	minPasswordLen = 10
)

const (
	sessionTTL   = 12 * time.Hour
	maxFailures  = 5
	failureWin   = 10 * time.Minute
	pbkdf2Iter   = 600_000
	pbkdf2KeyLen = 32
)

// hashIter 是 PBKDF2 轮数；测试里调低以免每个用例慢几百毫秒。
var hashIter = pbkdf2Iter

type Staff struct {
	ID           int64
	Username     string
	Role         Role
	PasswordHash string
	Disabled     bool
	CreatedAt    time.Time
}

// StaffRepo 是运营人员与会话的存储端口。
type StaffRepo interface {
	CreateStaff(ctx context.Context, s Staff) (Staff, error) // 用户名重复返回 ErrConflict
	StaffByName(ctx context.Context, username string) (Staff, error)
	StaffByID(ctx context.Context, id int64) (Staff, error)
	Staffs(ctx context.Context) ([]Staff, error)
	UpdateStaff(ctx context.Context, s Staff) error
	CountStaff(ctx context.Context) (int, error)
	PutSession(ctx context.Context, tokenHash string, staffID int64, expires time.Time) error
	SessionStaff(ctx context.Context, tokenHash string, now time.Time) (Staff, error) // 过期或不存在返回 ErrNotFound
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteSessionsOf(ctx context.Context, staffID int64) error
}

// Auth 管运营人员的登录、会话和账号。
type Auth struct {
	repo StaffRepo
	now  func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time // 用户名 → 最近失败的时间
}

func NewAuth(repo StaffRepo, now func() time.Time) *Auth {
	if now == nil {
		now = time.Now
	}
	return &Auth{repo: repo, now: now, failures: map[string][]time.Time{}}
}

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, hashIter, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("pbkdf2: %w", err)
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", hashIter, base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func checkPassword(pw, enc string) bool {
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func tokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func (a *Auth) blocked(user string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := a.now().Add(-failureWin)
	kept := a.failures[user][:0]
	for _, t := range a.failures[user] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(a.failures, user)
		return false
	}
	a.failures[user] = kept
	return len(kept) >= maxFailures
}

func (a *Auth) fail(user string) {
	a.mu.Lock()
	a.failures[user] = append(a.failures[user], a.now())
	a.mu.Unlock()
}

// dummyHash 让"用户不存在"也花同样的时间做一次 PBKDF2，避免靠响应时间枚举用户名。
var dummyHash = sync.OnceValue(func() string { h, _ := hashPassword("dummy-password"); return h })

// Login 校验口令并发一个新会话，返回明文令牌（只此一次）。
func (a *Auth) Login(ctx context.Context, username, password string) (string, Staff, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if a.blocked(username) {
		return "", Staff{}, ErrTooManyTry
	}
	st, err := a.repo.StaffByName(ctx, username)
	switch {
	case errors.Is(err, ErrNotFound):
		checkPassword(password, dummyHash())
		a.fail(username)
		return "", Staff{}, ErrBadLogin
	case err != nil:
		return "", Staff{}, fmt.Errorf("login: %w", err)
	}
	if !checkPassword(password, st.PasswordHash) || st.Disabled {
		a.fail(username)
		return "", Staff{}, ErrBadLogin
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Staff{}, fmt.Errorf("token: %w", err)
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	if err := a.repo.PutSession(ctx, tokenHash(tok), st.ID, a.now().Add(sessionTTL)); err != nil {
		return "", Staff{}, fmt.Errorf("login: %w", err)
	}
	return tok, st, nil
}

// Authenticate 用令牌找到人；停用的账号即使会话未过期也不通过。
func (a *Auth) Authenticate(ctx context.Context, token string) (Staff, error) {
	if token == "" {
		return Staff{}, ErrUnauth
	}
	st, err := a.repo.SessionStaff(ctx, tokenHash(token), a.now())
	if errors.Is(err, ErrNotFound) {
		return Staff{}, ErrUnauth
	}
	if err != nil {
		return Staff{}, fmt.Errorf("authenticate: %w", err)
	}
	if st.Disabled {
		return Staff{}, ErrUnauth
	}
	return st, nil
}

func (a *Auth) Logout(ctx context.Context, token string) error {
	return a.repo.DeleteSession(ctx, tokenHash(token))
}

func checkNewPassword(pw string) error {
	if len([]rune(pw)) < minPasswordLen {
		return invalid("password must be at least %d characters", minPasswordLen)
	}
	return nil
}

// Create 新建账号，只有 admin 可以。
func (a *Auth) Create(ctx context.Context, by Staff, username, password string, role Role) (Staff, error) {
	if by.Role != RoleAdmin {
		return Staff{}, ErrForbidden
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernameRE.MatchString(username) {
		return Staff{}, invalid("username must be 3-32 of a-z 0-9 . _ -")
	}
	if !role.Valid() {
		return Staff{}, invalid("unknown role %q", role)
	}
	if err := checkNewPassword(password); err != nil {
		return Staff{}, err
	}
	h, err := hashPassword(password)
	if err != nil {
		return Staff{}, err
	}
	return a.repo.CreateStaff(ctx, Staff{Username: username, Role: role, PasswordHash: h, CreatedAt: a.now()})
}

// Bootstrap 在库里一个账号都没有时创建首个管理员。
func (a *Auth) Bootstrap(ctx context.Context, username, password string) (bool, error) {
	n, err := a.repo.CountStaff(ctx)
	if err != nil {
		return false, fmt.Errorf("bootstrap: %w", err)
	}
	if n > 0 || username == "" {
		return false, nil
	}
	if _, err := a.Create(ctx, Staff{Role: RoleAdmin}, username, password, RoleAdmin); err != nil {
		return false, fmt.Errorf("bootstrap: %w", err)
	}
	return true, nil
}

func (a *Auth) List(ctx context.Context, by Staff) ([]Staff, error) {
	if by.Role != RoleAdmin {
		return nil, ErrForbidden
	}
	return a.repo.Staffs(ctx)
}

// Update 改角色与启停，并立即让该账号的会话失效。不能停用或降级自己，免得把后台锁死。
func (a *Auth) Update(ctx context.Context, by Staff, id int64, role Role, disabled bool) error {
	if by.Role != RoleAdmin {
		return ErrForbidden
	}
	if !role.Valid() {
		return invalid("unknown role %q", role)
	}
	if id == by.ID && (disabled || role != RoleAdmin) {
		return invalid("you cannot disable or demote yourself")
	}
	st, err := a.repo.StaffByID(ctx, id)
	if err != nil {
		return fmt.Errorf("update staff %d: %w", id, err)
	}
	st.Role, st.Disabled = role, disabled
	if err := a.repo.UpdateStaff(ctx, st); err != nil {
		return fmt.Errorf("update staff %d: %w", id, err)
	}
	if disabled {
		return a.repo.DeleteSessionsOf(ctx, id)
	}
	return nil
}

// ChangePassword 本人改口令，需旧口令；成功后其他会话全部失效。
func (a *Auth) ChangePassword(ctx context.Context, by Staff, oldPW, newPW string) error {
	st, err := a.repo.StaffByID(ctx, by.ID)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if !checkPassword(oldPW, st.PasswordHash) {
		return ErrBadLogin
	}
	if err := checkNewPassword(newPW); err != nil {
		return err
	}
	if st.PasswordHash, err = hashPassword(newPW); err != nil {
		return err
	}
	if err := a.repo.UpdateStaff(ctx, st); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	return a.repo.DeleteSessionsOf(ctx, by.ID)
}
