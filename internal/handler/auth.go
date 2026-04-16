package handler

import (
	"net/http"
	"strings"
	"time"

	"proxy-panel/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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

	if req.Username != h.cfg.Auth.AdminUser {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
		return
	}

	// 支持 bcrypt hash 和明文两种方式（兼容旧配置）
	storedPass := h.cfg.Auth.AdminPass
	if strings.HasPrefix(storedPass, "$2") {
		// bcrypt hash
		if err := bcrypt.CompareHashAndPassword([]byte(storedPass), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
			return
		}
	} else {
		// 明文比较（向后兼容，建议升级）
		if storedPass != req.Password {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
			return
		}
	}

	// 生成 JWT
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
