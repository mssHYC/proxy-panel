package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
)

type SubscriptionTokenService struct {
	db *database.DB
}

func NewSubscriptionTokenService(db *database.DB) *SubscriptionTokenService {
	return &SubscriptionTokenService{db: db}
}

// GenerateToken 32 字节 crypto/rand → base64url 无填充（长度 43）。
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const selectTokenCols = `id, user_id, name, token, enabled, expires_at,
	ip_bind_enabled, COALESCE(bound_ip,''), COALESCE(last_ip,''), COALESCE(last_ua,''),
	last_used_at, use_count, created_at`

func (s *SubscriptionTokenService) ListByUser(userID int64) ([]model.SubscriptionToken, error) {
	rows, err := s.db.Query(`SELECT `+selectTokenCols+`
		FROM subscription_tokens WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SubscriptionToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *SubscriptionTokenService) GetByToken(token string) (*model.SubscriptionToken, error) {
	row := s.db.QueryRow(`SELECT `+selectTokenCols+` FROM subscription_tokens WHERE token = ?`, token)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (s *SubscriptionTokenService) GetByID(id int64) (*model.SubscriptionToken, error) {
	row := s.db.QueryRow(`SELECT `+selectTokenCols+` FROM subscription_tokens WHERE id = ?`, id)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

type CreateTokenReq struct {
	Name          string     `json:"name" binding:"required"`
	ExpiresAt     *time.Time `json:"expires_at"`
	IPBindEnabled *bool      `json:"ip_bind_enabled"`
}

func (s *SubscriptionTokenService) Create(userID int64, req *CreateTokenReq) (*model.SubscriptionToken, error) {
	tok, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	bind := true
	if req.IPBindEnabled != nil {
		bind = *req.IPBindEnabled
	}
	res, err := s.db.Exec(`INSERT INTO subscription_tokens
		(user_id, name, token, enabled, expires_at, ip_bind_enabled, created_at)
		VALUES (?, ?, ?, 1, ?, ?, CURRENT_TIMESTAMP)`,
		userID, req.Name, tok, req.ExpiresAt, boolToInt(bind))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetByID(id)
}

type UpdateTokenReq struct {
	Name          *string    `json:"name"`
	Enabled       *bool      `json:"enabled"`
	ExpiresAt     *time.Time `json:"expires_at"`
	ExpiresNull   bool       `json:"expires_at_null"`
	IPBindEnabled *bool      `json:"ip_bind_enabled"`
	ResetBind     bool       `json:"reset_bind"`
}

func (s *SubscriptionTokenService) Update(id int64, req *UpdateTokenReq) (*model.SubscriptionToken, error) {
	cur, err := s.GetByID(id)
	if err != nil || cur == nil {
		return nil, err
	}
	sets := []string{}
	args := []any{}
	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*req.Enabled))
	}
	if req.ExpiresNull {
		sets = append(sets, "expires_at = NULL")
	} else if req.ExpiresAt != nil {
		sets = append(sets, "expires_at = ?")
		args = append(args, *req.ExpiresAt)
	}
	if req.IPBindEnabled != nil {
		sets = append(sets, "ip_bind_enabled = ?")
		args = append(args, boolToInt(*req.IPBindEnabled))
	}
	if req.ResetBind {
		sets = append(sets, "bound_ip = NULL")
	}
	if len(sets) == 0 {
		return cur, nil
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE subscription_tokens SET %s WHERE id = ?", strings.Join(sets, ", "))
	if _, err := s.db.Exec(q, args...); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *SubscriptionTokenService) Rotate(id int64) (*model.SubscriptionToken, error) {
	tok, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(
		`UPDATE subscription_tokens SET token = ?, bound_ip = NULL WHERE id = ?`,
		tok, id); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *SubscriptionTokenService) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM subscription_tokens WHERE id = ?`, id)
	return err
}

type tokenScanner interface {
	Scan(dest ...any) error
}

func scanToken(r tokenScanner) (*model.SubscriptionToken, error) {
	var t model.SubscriptionToken
	var enabled, ipBind int
	var expiresAt, lastUsedAt sql.NullTime
	err := r.Scan(&t.ID, &t.UserID, &t.Name, &t.Token, &enabled, &expiresAt,
		&ipBind, &t.BoundIP, &t.LastIP, &t.LastUA, &lastUsedAt, &t.UseCount, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	t.IPBindEnabled = ipBind != 0
	if expiresAt.Valid {
		v := expiresAt.Time
		t.ExpiresAt = &v
	}
	if lastUsedAt.Valid {
		v := lastUsedAt.Time
		t.LastUsedAt = &v
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenDisabled = errors.New("token disabled")
	ErrTokenExpired  = errors.New("token expired")
	ErrTokenIPBound  = errors.New("token bound to other ip")
)

// Validate 按 token 字符串完成校验，必要时原子绑定首访 IP。
func (s *SubscriptionTokenService) Validate(tokenStr, clientIP string) (*model.SubscriptionToken, error) {
	t, err := s.GetByToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTokenNotFound
	}
	if !t.Enabled {
		return nil, ErrTokenDisabled
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	if !t.IPBindEnabled {
		return t, nil
	}
	if t.BoundIP == "" {
		res, err := s.db.Exec(
			`UPDATE subscription_tokens SET bound_ip = ? WHERE id = ? AND bound_ip IS NULL`,
			clientIP, t.ID)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			t.BoundIP = clientIP
			return t, nil
		}
		t, err = s.GetByID(t.ID)
		if err != nil {
			return nil, err
		}
	}
	if t.BoundIP != clientIP {
		return nil, ErrTokenIPBound
	}
	return t, nil
}

// TouchAsync 记录审计元信息，失败仅打日志不影响响应。
func (s *SubscriptionTokenService) TouchAsync(id int64, ip, ua string) {
	go func() {
		_, _ = s.db.Exec(
			`UPDATE subscription_tokens
			 SET last_ip = ?, last_ua = ?, last_used_at = CURRENT_TIMESTAMP, use_count = use_count + 1
			 WHERE id = ?`, ip, ua, id)
	}()
}
