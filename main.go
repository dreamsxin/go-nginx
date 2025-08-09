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
	var configPath string
	var testConfig bool
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.BoolVar(&testConfig, "t", false, "Test configuration and exit")
	flag.BoolVar(&testConfig, "test-config", false, "Test configuration and exit")
	flag.Parse()
	if testConfig {
		_, err := config.LoadConfig(configPath)
		if err != nil {
			log.Fatalf("Configuration test failed: %v", err)
		}
		log.Println("Configuration file test is successful")
		os.Exit(0)
	}
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
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-signalChan
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			// 原有退出逻辑
			slog.Info("Shutting down server...")
			server.Stop()

			// 优雅关闭监控指标服务
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(ctx); err != nil {
				log.Fatalf("Metrics server forced to shutdown: %v", err)
			}
			log.Printf("Metrics server exiting")
			slog.Info("Server exiting")
			return
		case syscall.SIGHUP:
			// 新增: 配置重载逻辑
			slog.Info("Received SIGHUP, reloading configuration...")
			newCfg, err := config.LoadConfig(configPath)
			if err != nil {
				slog.Error(fmt.Sprintf("Failed to reload config: %v", err))
				continue
			}
			if err := server.ReloadConfig(newCfg); err != nil {
				slog.Error(fmt.Sprintf("Failed to apply new config: %v", err))
			} else {
				slog.Info("Configuration reloaded successfully")
			}
		}
	}
}
