package router

import (
	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"
	notify "proxy-panel/internal/service/notify"

	"github.com/gin-gonic/gin"
)

// Setup 初始化路由，注册所有端点
func Setup(cfg *config.Config, db *database.DB, mgr *kernel.Manager,
	userSvc *service.UserService, nodeSvc *service.NodeService,
	trafficSvc *service.TrafficService, notifySvc *notify.NotifyService) *gin.Engine {

	r := gin.Default()

	// 静态文件 (优先使用 web/dist，兼容部署目录 web/)
	r.Static("/assets", "./web/dist/assets")
	r.StaticFile("/favicon.svg", "./web/dist/favicon.svg")
	r.StaticFile("/", "./web/dist/index.html")
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/dist/index.html")
	})

	// 初始化 Handlers
	authHandler := handler.NewAuthHandler(cfg)
	userHandler := handler.NewUserHandler(userSvc)
	nodeHandler := handler.NewNodeHandler(nodeSvc)
	dashboardHandler := handler.NewDashboardHandler(userSvc, nodeSvc, trafficSvc, mgr, db)
	kernelHandler := handler.NewKernelHandler(mgr)
	trafficHandler := handler.NewTrafficHandler(trafficSvc)
	settingHandler := handler.NewSettingHandler(db)
	notifyHandler := handler.NewNotifyHandler(notifySvc)
	subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc)

	// 限流器
	rateLimiter := NewRateLimiter()
	subLimiter := NewSubRateLimiter()

	api := r.Group("/api")
	{
		// 公开端点
		api.POST("/auth/login", rateLimiter.LoginRateLimit(), authHandler.Login)
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

			// 通知测试
			auth.POST("/notify/test", notifyHandler.Test)
		}
	}

	return r
}
