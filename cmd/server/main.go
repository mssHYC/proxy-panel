package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/router"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/firewall"
	notify "proxy-panel/internal/service/notify"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	resetPass := flag.String("reset-pass", "", "重置管理员密码后退出（供 install.sh reset-pwd 调用）")
	disableTOTP := flag.Bool("disable-totp", false, "强制关闭 2FA 后退出（供 install.sh disable-2fa 应急解锁调用）")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 安全守卫：默认凭证 / JWT 密钥未改动时拒绝启动，避免生产裸奔
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v\n请编辑 %s 后重新启动", err, *cfgPath)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// CLI 一次性操作：重置密码后立即退出，不进入服务监听流程
	// 直接更新 DB 并 bump token_version，使所有历史 JWT 立即失效
	if *resetPass != "" {
		authSvc := service.NewAuthService(db, cfg)
		if err := authSvc.ForceResetPassword(*resetPass); err != nil {
			log.Fatalf("重置密码失败: %v", err)
		}
		log.Println("密码已重置，所有现有登录会话已失效")
		os.Exit(0)
	}

	// CLI 一次性操作：丢失 authenticator 设备时应急关闭 2FA 后退出
	if *disableTOTP {
		authSvc := service.NewAuthService(db, cfg)
		if err := authSvc.ForceDisableTOTP(); err != nil {
			log.Fatalf("关闭 2FA 失败: %v", err)
		}
		log.Println("2FA 已关闭，所有现有登录会话已失效")
		os.Exit(0)
	}

	// 启动时用 settings 表覆盖 yaml 中的调度相关配置（reset_cron/collect_interval/warn_percent）
	// 这样后台"告警设置"修改后的值在进程重启后也能生效
	if err := service.ApplySettingsToConfig(db, cfg); err != nil {
		log.Printf("加载 settings 覆盖失败，继续使用 yaml 默认值: %v", err)
	}

	// 初始化内核 Manager
	mgr := kernel.NewManager()
	xrayEngine := kernel.NewXrayEngine(cfg.Kernel.XrayPath, cfg.Kernel.XrayConfig, cfg.Kernel.XrayAPIPort)
	mgr.Register(xrayEngine)
	singboxEngine := kernel.NewSingboxEngine(cfg.Kernel.SingboxPath, cfg.Kernel.SingboxConfig, cfg.Kernel.SingboxAPIPort)
	mgr.Register(singboxEngine)

	// 初始化 Services
	authSvc := service.NewAuthService(db, cfg)
	userSvc := service.NewUserService(db)
	trafficSvc := service.NewTrafficService(db, mgr)
	notifySvc := notify.NewNotifyService(cfg, db)
	fwSvc, err := firewall.NewService(cfg.Firewall, notifySvc)
	if err != nil {
		log.Printf("防火墙服务初始化失败，已降级为关闭状态: %v", err)
	}
	nodeSvc := service.NewNodeService(db, fwSvc)

	// 设置服务器流量限额
	if cfg.Traffic.ServerLimitGB > 0 {
		if err := trafficSvc.SetServerLimit(int64(cfg.Traffic.ServerLimitGB)); err != nil {
			log.Printf("设置服务器流量限额失败: %v", err)
		}
	}

	// 启动时同步内核配置（确保 Xray/Sing-box 配置与数据库一致）
	syncSvc := service.NewKernelSyncService(db, mgr)
	if err := syncSvc.Sync(); err != nil {
		log.Printf("启动时同步内核配置失败: %v", err)
	}

	// 节点健康检查
	healthSvc := service.NewHealthChecker(db, nodeSvc, notifySvc)

	// 审计日志
	auditSvc := service.NewAuditService(db)

	// 初始化调度器（流量超限/用户过期时通过 syncSvc 立即剔除用户并重启内核）
	scheduler := service.NewScheduler(cfg, trafficSvc, notifySvc, db, syncSvc, healthSvc)

	// 启动时对存量 enable 节点做一次单向 ensure（幂等）
	if fwSvc.Enabled() {
		go func() {
			nodes, err := nodeSvc.ListEnabled()
			if err != nil {
				log.Printf("[防火墙] 启动对齐：读取节点失败: %v", err)
				return
			}
			ports := make([]int, 0, len(nodes))
			for _, n := range nodes {
				ports = append(ports, n.Port)
			}
			fwSvc.EnsureAll(context.Background(), ports)
			log.Printf("[防火墙] 启动对齐完成，处理 %d 个节点端口", len(ports))
		}()
	}

	scheduler.Start()
	defer scheduler.Stop()

	// 设置路由
	r := router.Setup(cfg, db, mgr, userSvc, nodeSvc, trafficSvc, notifySvc, authSvc, scheduler, fwSvc, auditSvc, cfg.Database.Path)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if cfg.Server.Domain != "" {
		log.Printf("ProxyPanel 启动成功，域名: %s，监听 %s", cfg.Server.Domain, addr)
	} else {
		log.Printf("ProxyPanel 启动成功，监听 %s", addr)
	}
	if cfg.Server.TLS && cfg.Server.Cert != "" && cfg.Server.Key != "" {
		log.Printf("TLS 已启用，证书: %s", cfg.Server.Cert)
		if err := r.RunTLS(addr, cfg.Server.Cert, cfg.Server.Key); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	} else {
		if err := r.Run(addr); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}
}
