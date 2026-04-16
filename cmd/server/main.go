package main

import (
	"flag"
	"log"

	"proxy-panel/internal/config"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("ProxyPanel 启动中，端口: %d", cfg.Server.Port)
}
