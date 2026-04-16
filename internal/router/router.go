package router

import (
	"io/fs"
	"net/http"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"
	notify "proxy-panel/internal/service/notify"
	"proxy-panel/web"

	"github.com/gin-gonic/gin"
)

// Setup 初始化路由，注册所有端点
func Setup(cfg *config.Config, db *database.DB, mgr *kernel.Manager,
	userSvc *service.UserService, nodeSvc *service.NodeService,
	trafficSvc *service.TrafficService, notifySvc *notify.NotifyService,
	authSvc *service.AuthService) *gin.Engine {

	r := gin.Default()

	// 嵌入的前端静态文件
	distFS, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServer(http.FS(distFS))

	// 静态资源路由
	r.GET("/assets/*filepath", gin.WrapH(fileServer))
	r.GET("/favicon.svg", gin.WrapH(fileServer))
	r.GET("/icons.svg", gin.WrapH(fileServer))

	// SPA 回退：非 API 路由都返回 index.html
	r.NoRoute(func(c *gin.Context) {
		c.FileFromFS("index.html", http.FS(distFS))
	})

	// 初始化 Handlers
	authHandler := handler.NewAuthHandler(cfg, authSvc)
	userHandler := handler.NewUserHandler(userSvc)
	nodeHandler := handler.NewNodeHandler(nodeSvc)
	dashboardHandler := handler.NewDashboardHandler(userSvc, nodeSvc, trafficSvc, mgr, db)
	kernelHandler := handler.NewKernelHandler(mgr)
	trafficHandler := handler.NewTrafficHandler(trafficSvc)
	settingHandler := handler.NewSettingHandler(db, cfg)
	notifyHandler := handler.NewNotifyHandler(notifySvc)
	subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc, db)

	// 限流器
	rateLimiter := NewRateLimiter()
	subLimiter := NewSubRateLimiter()

	api := r.Group("/api")
	{
		// 公开端点
		api.POST("/auth/login", rateLimiter.LoginRateLimit(), authHandler.Login)
		api.POST("/auth/2fa/verify", rateLimiter.LoginRateLimit(), authHandler.Verify2FA)
		api.GET("/sub/:uuid", subLimiter.Limit(), subHandler.Subscribe)

		// 需要认证的端点
		auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret))
		{
			// 仪表盘
			auth.GET("/dashboard", dashboardHandler.Get)

			// 用户管理
			auth.GET("/users", userHandler.List)
			auth.POST("/users", userHandler.Create)
			auth.GET("/users/:id", userHandler.Get)
			auth.PUT("/users/:id", userHandler.Update)
			auth.DELETE("/users/:id", userHandler.Delete)
			auth.POST("/users/:id/reset-traffic", userHandler.ResetTraffic)
			auth.POST("/users/:id/reset-uuid", userHandler.ResetUUID)

			// 节点管理
			auth.GET("/nodes", nodeHandler.List)
			auth.POST("/nodes", nodeHandler.Create)
			auth.GET("/nodes/:id", nodeHandler.Get)
			auth.PUT("/nodes/:id", nodeHandler.Update)
			auth.DELETE("/nodes/:id", nodeHandler.Delete)

			// 内核管理
			auth.GET("/kernel/status", kernelHandler.Status)
			auth.POST("/kernel/restart", kernelHandler.Restart)

			// 流量管理
			auth.GET("/traffic/server", trafficHandler.GetServerTraffic)
			auth.POST("/traffic/server/limit", trafficHandler.SetServerLimit)
			auth.GET("/traffic/history", trafficHandler.GetHistory)

			// 系统设置
			auth.GET("/settings", settingHandler.Get)
			auth.PUT("/settings", settingHandler.Update)

			// 账号管理
			auth.PUT("/auth/password", authHandler.ChangePassword)
			auth.PUT("/auth/username", authHandler.ChangeUsername)
			auth.GET("/auth/2fa/status", authHandler.Get2FAStatus)
			auth.POST("/auth/2fa/setup", authHandler.Setup2FA)
			auth.POST("/auth/2fa/enable", authHandler.Enable2FA)
			auth.POST("/auth/2fa/disable", authHandler.Disable2FA)

			// 通知测试
			auth.POST("/notify/test", notifyHandler.Test)
		}
	}

	return r
}
