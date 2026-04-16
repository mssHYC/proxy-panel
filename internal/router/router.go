package router

import (
	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/service"

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

	userSvc := service.NewUserService(db)
	userHandler := handler.NewUserHandler(userSvc)
	nodeSvc := service.NewNodeService(db)
	nodeHandler := handler.NewNodeHandler(nodeSvc)

	api := r.Group("/api")
	{
		api.POST("/auth/login", rateLimiter.LoginRateLimit(), authHandler.Login)
		// 订阅端点
		subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc)
		api.GET("/sub/:uuid", subLimiter.Limit(), subHandler.Subscribe)
		// 需要认证的端点
		auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret))
		{
			auth.GET("/users", userHandler.List)
			auth.POST("/users", userHandler.Create)
			auth.GET("/users/:id", userHandler.Get)
			auth.PUT("/users/:id", userHandler.Update)
			auth.DELETE("/users/:id", userHandler.Delete)
			auth.POST("/users/:id/reset-traffic", userHandler.ResetTraffic)

			auth.GET("/nodes", nodeHandler.List)
			auth.POST("/nodes", nodeHandler.Create)
			auth.GET("/nodes/:id", nodeHandler.Get)
			auth.PUT("/nodes/:id", nodeHandler.Update)
			auth.DELETE("/nodes/:id", nodeHandler.Delete)
		}
	}

	return r
}
