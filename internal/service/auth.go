package service

import (
	"fmt"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

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

// VerifyPassword 验证密码
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
	return s.setSetting("admin_pass", string(hashed))
}

// ChangeUsername 修改用户名
func (s *AuthService) ChangeUsername(password, newUsername string) error {
	if !s.VerifyPassword(password) {
		return fmt.Errorf("密码错误")
	}
	if len(newUsername) < 3 {
		return fmt.Errorf("用户名长度至少 3 位")
	}
	return s.setSetting("admin_user", newUsername)
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
	return nil
}

// DisableTOTP 关闭 2FA
func (s *AuthService) DisableTOTP(password string) error {
	if !s.VerifyPassword(password) {
		return fmt.Errorf("密码错误")
	}
	s.setSetting("totp_enabled", "false")
	s.setSetting("totp_secret", "")
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
