package handler

import (
	"fmt"
	"net/http"
	"time"

	"proxy-panel/internal/config"
	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	cfg     *config.Config
	authSvc *service.AuthService
}

func NewAuthHandler(cfg *config.Config, authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{cfg: cfg, authSvc: authSvc}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	// 验证用户名
	if req.Username != h.authSvc.GetUsername() {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
		return
	}

	// 验证密码
	if !h.authSvc.VerifyPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
		return
	}

	// 检查是否启用 2FA
	if h.authSvc.IsTOTPEnabled() {
		// 生成临时 token（短期有效，仅用于 2FA 验证）
		tempToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username": req.Username,
			"type":     "2fa_pending",
			"exp":      time.Now().Add(5 * time.Minute).Unix(),
		})
		tempTokenStr, _ := tempToken.SignedString([]byte(h.cfg.Auth.JWTSecret))
		c.JSON(http.StatusOK, gin.H{"require_2fa": true, "temp_token": tempTokenStr})
		return
	}

	// 无 2FA，直接签发正式 token
	c.JSON(http.StatusOK, gin.H{"token": h.generateToken(req.Username)})
}

// Verify2FA 二次验证
func (h *AuthHandler) Verify2FA(c *gin.Context) {
	var req struct {
		TempToken string `json:"temp_token" binding:"required"`
		Code      string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	// 验证临时 token
	token, err := jwt.Parse(req.TempToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("invalid signing method")
		}
		return []byte(h.cfg.Auth.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "临时令牌无效或已过期", "code": "ERR_INVALID_TOKEN"})
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["type"] != "2fa_pending" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的令牌类型", "code": "ERR_INVALID_TOKEN"})
		return
	}

	// 验证 TOTP code
	if !h.authSvc.ValidateTOTP(req.Code) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误", "code": "ERR_2FA_FAILED"})
		return
	}

	username, _ := claims["username"].(string)
	c.JSON(http.StatusOK, gin.H{"token": h.generateToken(username)})
}

// ChangePassword 修改密码
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	if err := h.authSvc.ChangePassword(req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_CHANGE_PASSWORD"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// ChangeUsername 修改用户名
func (h *AuthHandler) ChangeUsername(c *gin.Context) {
	var req struct {
		Password    string `json:"password" binding:"required"`
		NewUsername string `json:"new_username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	if err := h.authSvc.ChangeUsername(req.Password, req.NewUsername); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_CHANGE_USERNAME"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户名修改成功"})
}

// Get2FAStatus 查询 2FA 状态
func (h *AuthHandler) Get2FAStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": h.authSvc.IsTOTPEnabled()})
}

// Setup2FA 生成 TOTP 密钥
func (h *AuthHandler) Setup2FA(c *gin.Context) {
	secret, url, err := h.authSvc.SetupTOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"secret": secret, "qr_url": url})
}

// Enable2FA 验证并启用 2FA
func (h *AuthHandler) Enable2FA(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.authSvc.EnableTOTP(req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "二次验证已启用"})
}

// Disable2FA 关闭 2FA
func (h *AuthHandler) Disable2FA(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := h.authSvc.DisableTOTP(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "二次验证已关闭"})
}

func (h *AuthHandler) generateToken(username string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": username,
		"type":     "access",
		"exp":      time.Now().Add(time.Duration(h.cfg.Auth.TokenExpiry) * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(h.cfg.Auth.JWTSecret))
	return tokenStr
}
