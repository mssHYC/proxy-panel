package main

import (
	"flag"
	"fmt"
	"log"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/router"
	"proxy-panel/internal/service"
	notify "proxy-panel/internal/service/notify"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
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
	singboxEngine := kernel.NewSingboxEngine(cfg.Kernel.SingboxPath, cfg.Kernel.SingboxConfig)
	mgr.Register(singboxEngine)

	// 初始化 Services
	userSvc := service.NewUserService(db)
	nodeSvc := service.NewNodeService(db)
	trafficSvc := service.NewTrafficService(db, mgr)
	notifySvc := notify.NewNotifyService(cfg, db)

	// 设置服务器流量限额
	if cfg.Traffic.ServerLimitGB > 0 {
		if err := trafficSvc.SetServerLimit(int64(cfg.Traffic.ServerLimitGB)); err != nil {
			log.Printf("设置服务器流量限额失败: %v", err)
		}
	}

	// 初始化调度器
	scheduler := service.NewScheduler(cfg, trafficSvc, notifySvc, db)
	scheduler.Start()
	defer scheduler.Stop()

	// 设置路由
	r := router.Setup(cfg, db, mgr, userSvc, nodeSvc, trafficSvc, notifySvc)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("ProxyPanel 启动成功，监听 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
