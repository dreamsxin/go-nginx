package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/core"
	"github.com/dreamsxin/go-nginx/monitor"
	"github.com/dreamsxin/go-nginx/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	// 初始化监控组件
	metrics := monitor.NewMetrics()
	statsManager := monitor.NewStatsManager()

	// 创建服务器实例
	server := core.NewServer(cfg, metrics, statsManager)

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// 启动监控指标服务
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats := statsManager.GetAllBackendStats()
		json.NewEncoder(w).Encode(stats)
	}))
	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: metricsMux,
	}
	go func() {
		log.Printf("Metrics server starting on %s", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server failed to start: %v", err)
		}
	}()

	// 等待中断信号以优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	// 停止服务器
	server.Stop()

	// 优雅关闭监控指标服务
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Fatalf("Metrics server forced to shutdown: %v", err)
	}
	log.Printf("Metrics server exiting")
	slog.Info("Server exiting")
}
