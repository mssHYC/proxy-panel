package main

import (
	"context"
	"flag"
	"fmt"
	"log"

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

	// 初始化调度器（流量超限/用户过期时通过 syncSvc 立即剔除用户并重启内核）
	scheduler := service.NewScheduler(cfg, trafficSvc, notifySvc, db, syncSvc)

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
	r := router.Setup(cfg, db, mgr, userSvc, nodeSvc, trafficSvc, notifySvc, authSvc)

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
