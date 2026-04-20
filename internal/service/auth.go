package service

import (
	"crypto/subtle"
	"fmt"
	"strconv"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// 用于在没有真实 hash / 用户名不匹配时仍执行一次 bcrypt，消除登录响应时间差
// 值为 bcrypt("__no_match__")，任意密码都将比较失败
const dummyBcryptHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

type AuthService struct {
	db  *database.DB
	cfg *config.Config
}

func NewAuthService(db *database.DB, cfg *config.Config) *AuthService {
	svc := &AuthService{db: db, cfg: cfg}
	svc.initFromConfig()
	return svc
}

// initFromConfig 首次启动时将 config 中的凭据写入数据库（如果数据库中还没有）
func (s *AuthService) initFromConfig() {
	if _, err := s.getSetting("admin_user"); err != nil {
		s.setSetting("admin_user", s.cfg.Auth.AdminUser)
	}
	if _, err := s.getSetting("admin_pass"); err != nil {
		// 如果 config 中是明文，转为 bcrypt
		pass := s.cfg.Auth.AdminPass
		if len(pass) > 0 && pass[0] != '$' {
			hashed, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
			pass = string(hashed)
		}
		s.setSetting("admin_pass", pass)
	}
	if _, err := s.getSetting("totp_enabled"); err != nil {
		s.setSetting("totp_enabled", "false")
	}
	if _, err := s.getSetting("token_version"); err != nil {
		s.setSetting("token_version", "1")
	}
}

func (s *AuthService) getSetting(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	return val, err
}

func (s *AuthService) setSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=?", key, value, value)
	return err
}

// GetUsername 获取当前管理员用户名
func (s *AuthService) GetUsername() string {
	val, err := s.getSetting("admin_user")
	if err != nil {
		return s.cfg.Auth.AdminUser
	}
	return val
}

// GetPasswordHash 获取当前密码 hash
func (s *AuthService) GetPasswordHash() string {
	val, err := s.getSetting("admin_pass")
	if err != nil {
		return s.cfg.Auth.AdminPass
	}
	return val
}

// GetTokenVersion 当前 token 版本号；所有已签发 JWT 的 ver claim 与此不一致即视为已吊销
func (s *AuthService) GetTokenVersion() int {
	val, err := s.getSetting("token_version")
	if err != nil {
		return 1
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// bumpTokenVersion 递增 token 版本号，使历史 JWT 全部作废
// 任何足以改变账号安全态势的操作（改密码/改用户名/开关 2FA）都应调用
func (s *AuthService) bumpTokenVersion() {
	next := s.GetTokenVersion() + 1
	s.setSetting("token_version", strconv.Itoa(next))
}

// VerifyCredentials 恒时校验用户名 + 密码
// - 即便用户名不匹配，仍执行一次 bcrypt，消除 Login 响应时间侧信道
// - 用户名使用 subtle.ConstantTimeCompare 比较
// 返回值仅指示整体是否通过，调用方不应据此区分"用户名错误"与"密码错误"
func (s *AuthService) VerifyCredentials(username, password string) bool {
	expectedUser := s.GetUsername()
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUser)) == 1

	stored := s.GetPasswordHash()
	// 用户名不匹配时改用 dummy hash，保证两条分支都走 bcrypt
	if !userOK {
		stored = dummyBcryptHash
	}

	var passOK bool
	if len(stored) > 0 && stored[0] == '$' {
		passOK = bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	} else {
		// 历史明文兜底；此时也预执行一次 bcrypt，避免时间差
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
		passOK = subtle.ConstantTimeCompare([]byte(stored), []byte(password)) == 1
	}

	return userOK && passOK
}

// VerifyPassword 仅校验密码（管理员已登录场景，如改密/关 2FA 时复核）
func (s *AuthService) VerifyPassword(password string) bool {
	stored := s.GetPasswordHash()
	if len(stored) > 0 && stored[0] == '$' {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	return stored == password
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(oldPass, newPass string) error {
	if !s.VerifyPassword(oldPass) {
		return fmt.Errorf("旧密码错误")
	}
	if len(newPass) < 8 {
		return fmt.Errorf("新密码长度至少 8 位")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}
	if err := s.setSetting("admin_pass", string(hashed)); err != nil {
		return err
	}
	// 密码变更 → 吊销所有历史 token
	s.bumpTokenVersion()
	return nil
}

// ChangeUsername 修改用户名
func (s *AuthService) ChangeUsername(password, newUsername string) error {
	if !s.VerifyPassword(password) {
		return fmt.Errorf("密码错误")
	}
	if len(newUsername) < 3 {
		return fmt.Errorf("用户名长度至少 3 位")
	}
	if err := s.setSetting("admin_user", newUsername); err != nil {
		return err
	}
	s.bumpTokenVersion()
	return nil
}

// IsTOTPEnabled 查询 2FA 是否启用
func (s *AuthService) IsTOTPEnabled() bool {
	val, _ := s.getSetting("totp_enabled")
	return val == "true"
}

// SetupTOTP 生成 TOTP 密钥，返回 secret 和 otpauth URL（用于生成二维码）
func (s *AuthService) SetupTOTP() (secret string, url string, err error) {
	username := s.GetUsername()
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "ProxyPanel",
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("生成 TOTP 密钥失败: %w", err)
	}
	// 暂存 secret（尚未启用，等用户验证后才写入）
	s.setSetting("totp_secret_pending", key.Secret())
	return key.Secret(), key.URL(), nil
}

// EnableTOTP 验证 TOTP code 后启用 2FA
func (s *AuthService) EnableTOTP(code string) error {
	secret, err := s.getSetting("totp_secret_pending")
	if err != nil || secret == "" {
		return fmt.Errorf("请先调用 setup 生成密钥")
	}
	if !totp.Validate(code, secret) {
		return fmt.Errorf("验证码错误，请重试")
	}
	s.setSetting("totp_secret", secret)
	s.setSetting("totp_enabled", "true")
	s.setSetting("totp_secret_pending", "") // 清除临时密钥
	// 2FA 启用是安全态变更，顺势吊销历史 token（历史 token 未走 2FA 流程）
	s.bumpTokenVersion()
	return nil
}

// DisableTOTP 关闭 2FA
func (s *AuthService) DisableTOTP(password string) error {
	if !s.VerifyPassword(password) {
		return fmt.Errorf("密码错误")
	}
	s.setSetting("totp_enabled", "false")
	s.setSetting("totp_secret", "")
	// 关闭 2FA 降低了安全假设，吊销历史 token
	s.bumpTokenVersion()
	return nil
}

// ValidateTOTP 验证 TOTP code
func (s *AuthService) ValidateTOTP(code string) bool {
	secret, err := s.getSetting("totp_secret")
	if err != nil || secret == "" {
		return false
	}
	return totp.Validate(code, secret)
}
