package router

import (
	"io/fs"
	"net/http"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/firewall"
	notify "proxy-panel/internal/service/notify"
	"proxy-panel/web"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Setup 初始化路由，注册所有端点
func Setup(cfg *config.Config, db *database.DB, mgr *kernel.Manager,
	userSvc *service.UserService, nodeSvc *service.NodeService,
	trafficSvc *service.TrafficService, notifySvc *notify.NotifyService,
	authSvc *service.AuthService, scheduler *service.Scheduler,
	fwSvc *firewall.Service, auditSvc *service.AuditService, dbPath string) *gin.Engine {

	r := gin.Default()

	// 反代部署时必须配置 trusted_proxies，否则 X-Forwarded-For 可被伪造
	// 未配置时传 nil 让 gin 忽略所有代理 header，ClientIP() 回退到直连 RemoteAddr
	_ = r.SetTrustedProxies(cfg.Server.TrustedProxies)

	// Prometheus 指标中间件
	r.Use(MetricsMiddleware())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 域名绑定：配置了域名时，拒绝通过 IP 直接访问
	if cfg.Server.Domain != "" {
		r.Use(DomainGuard(cfg.Server.Domain))
	}

	// 嵌入的前端静态文件
	distFS, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServer(http.FS(distFS))

	// 静态资源路由
	r.GET("/assets/*filepath", gin.WrapH(fileServer))
	r.GET("/favicon.svg", gin.WrapH(fileServer))
	r.GET("/icons.svg", gin.WrapH(fileServer))

	// 预读 index.html 用于 SPA 回退
	indexHTML, _ := fs.ReadFile(web.DistFS, "dist/index.html")

	// SPA 回退：非 API 路由都返回 index.html
	r.NoRoute(func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	// 初始化内核同步服务
	syncSvc := service.NewKernelSyncService(db, mgr)

	// 初始化 Handlers
	authHandler := handler.NewAuthHandler(cfg, authSvc)
	userHandler := handler.NewUserHandler(userSvc, syncSvc)
	nodeHandler := handler.NewNodeHandler(nodeSvc, syncSvc)
	dashboardHandler := handler.NewDashboardHandler(userSvc, nodeSvc, trafficSvc, mgr, db)
	kernelHandler := handler.NewKernelHandler(mgr)
	trafficHandler := handler.NewTrafficHandler(trafficSvc)
	settingHandler := handler.NewSettingHandler(db, cfg, scheduler)
	notifyHandler := handler.NewNotifyHandler(notifySvc)
	subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc, db)
	firewallHandler := handler.NewFirewallHandler(fwSvc, cfg, db, nodeSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)
	backupHandler := handler.NewBackupHandler(db, dbPath)

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
		auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret, authSvc.GetTokenVersion), AuditMiddleware(auditSvc))
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
			auth.POST("/nodes/generate-reality-keypair", nodeHandler.GenerateRealityKeypair)

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

			// 防火墙管理
			auth.POST("/firewall/probe", firewallHandler.Probe)
			auth.POST("/firewall/apply", firewallHandler.Apply)

			// 审计日志
			auth.GET("/audit-logs", auditHandler.List)

			// 备份/恢复
			auth.GET("/backup/export", backupHandler.Export)
			auth.POST("/backup/import", backupHandler.Import)
		}
	}

	return r
}
