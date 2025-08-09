package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/core"
	"github.com/dreamsxin/go-nginx/util"
)

func main() {
	// 初始化日志
	util.InitLogger()
	log.Println("Starting go-nginx...")

	// 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

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
	util.LogInfof("Shutting down server...")

	// 停止服务器
	server.Stop()
	util.LogInfof("Server exiting")
}
