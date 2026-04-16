package router

import (
	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"

	"github.com/gin-gonic/gin"
)

func Setup(cfg *config.Config, db *database.DB) *gin.Engine {
	r := gin.Default()

	// 静态文件
	r.Static("/assets", "./web/assets")
	r.StaticFile("/", "./web/index.html")
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/index.html")
	})

	authHandler := handler.NewAuthHandler(cfg)
	rateLimiter := NewRateLimiter()
	subLimiter := NewSubRateLimiter()

	api := r.Group("/api")
	{
		api.POST("/auth/login", rateLimiter.LoginRateLimit(), authHandler.Login)
		// 订阅端点 (后续 task 注册)
		_ = subLimiter
		// 需要认证的端点
		auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret))
		{
			_ = auth // 后续 task 注册路由
		}
	}

	return r
}
