package handler

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"proxy-panel/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
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

	passHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Password)))
	adminHash := fmt.Sprintf("%x", sha256.Sum256([]byte(h.cfg.Auth.AdminPass)))

	if req.Username != h.cfg.Auth.AdminUser || passHash != adminHash {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": req.Username,
		"exp":      time.Now().Add(time.Duration(h.cfg.Auth.TokenExpiry) * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(h.cfg.Auth.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败", "code": "ERR_INTERNAL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenStr})
}
