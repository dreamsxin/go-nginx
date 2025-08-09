package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	balancerhttp "github.com/dreamsxin/go-nginx/balancer/http"
	"github.com/dreamsxin/go-nginx/balancer/tcp"
	"github.com/dreamsxin/go-nginx/balancer/udp"
	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/monitor"
)

// Server 服务器实例
type Server struct {
	config       *config.Config
	metrics      *monitor.Metrics
	statsManager *monitor.StatsManager
	httpBalancer *balancerhttp.Balancer
	httpServer   *http.Server
	tcpBalancer  *tcp.Balancer
	udpBalancer  *udp.Balancer
	wg           sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, metrics *monitor.Metrics, statsManager *monitor.StatsManager) *Server {
	return &Server{
		config:       cfg,
		metrics:      metrics,
		statsManager: statsManager,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 启动HTTP负载均衡器
	if s.config.Balancer.HTTP.Enabled {
		balancer, err := balancerhttp.NewBalancer(s.config.Balancer.HTTP, s.metrics, s.statsManager)
		if err != nil {
			return err
		}
		s.httpBalancer = balancer

		s.httpServer = &http.Server{
			Addr:    ":" + strconv.Itoa(s.config.Balancer.HTTP.Listen),
			Handler: balancer,
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			slog.Info(fmt.Sprintf("Starting HTTP balancer on port %d", s.config.Balancer.HTTP.Listen))
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error(fmt.Sprintf("HTTP balancer error: %v", err))
			}
		}()
	}

	// 启动TCP负载均衡器
	if s.config.Balancer.TCP.Enabled {
		balancer, err := tcp.NewBalancer(s.config.Balancer.TCP, s.metrics, s.statsManager)
		if err != nil {
			return err
		}
		s.tcpBalancer = balancer

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			slog.Info(fmt.Sprintf("Starting TCP balancer on port %d", s.config.Balancer.TCP.Listen))
			if err := balancer.ListenAndServe(); err != nil {
				slog.Error(fmt.Sprintf("TCP balancer error: %v", err))
			}
		}()
	}

	// 启动UDP负载均衡器
	if s.config.Balancer.UDP.Enabled {
		balancer, err := udp.NewBalancer(s.config.Balancer.UDP, s.metrics, s.statsManager)
		if err != nil {
			return err
		}
		s.udpBalancer = balancer

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			slog.Info(fmt.Sprintf("Starting UDP balancer on port %d", s.config.Balancer.UDP.Listen))
			if err := balancer.ListenAndServe(); err != nil {
				slog.Error(fmt.Sprintf("UDP balancer error: %v", err))
			}
		}()
	}

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() {
	if s.httpServer != nil {
		// 新增：优雅关闭HTTP服务器
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			slog.Error(fmt.Sprintf("HTTP server shutdown error: %v", err))
		}
	}
	if s.httpBalancer != nil {
		s.httpBalancer.Stop()
	}
	if s.tcpBalancer != nil {
		s.tcpBalancer.Stop()
	}
	if s.udpBalancer != nil {
		s.udpBalancer.Stop()
	}
	// 等待所有服务goroutine退出
	s.wg.Wait()
}

// ReloadConfig 平滑重载配置
func (s *Server) ReloadConfig(newConfig *config.Config) error {
	// 动态更新HTTP负载均衡器
	if s.httpBalancer != nil && newConfig.Balancer.HTTP.Enabled {
		if err := s.httpBalancer.UpdateConfig(newConfig.Balancer.HTTP); err != nil {
			return err
		}
	}

	// 更新TCP负载均衡器
	if s.tcpBalancer != nil && newConfig.Balancer.TCP.Enabled {
		if err := s.tcpBalancer.UpdateConfig(newConfig.Balancer.TCP); err != nil {
			return err
		}
	}

	// 更新UDP负载均衡器
	if s.udpBalancer != nil && newConfig.Balancer.UDP.Enabled {
		if err := s.udpBalancer.UpdateConfig(newConfig.Balancer.UDP); err != nil {
			return err
		}
	}

	s.config = newConfig
	return nil
}
