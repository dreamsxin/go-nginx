package main

import (
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/core"
	"github.com/dreamsxin/go-nginx/util"
)

func main() {
	// 新增：解析命令行参数
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化日志
	util.InitLogger(cfg)
	log.Println("Starting go-nginx...")

	// 创建服务器实例
	server := core.NewServer(cfg)

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// 等待中断信号以优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	// 停止服务器
	server.Stop()
	slog.Info("Server exiting")
}
