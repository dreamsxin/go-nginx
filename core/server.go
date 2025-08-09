package core

import (
	"net/http"
	"strconv"
	"sync"

	balancerhttp "github.com/dreamsxin/go-nginx/balancer/http"
	"github.com/dreamsxin/go-nginx/balancer/tcp"
	"github.com/dreamsxin/go-nginx/balancer/udp"
	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/util"
)

// Server 服务器实例
type Server struct {
	config *config.Config
	// 添加HTTP服务器实例引用
	httpBalancer *balancerhttp.Balancer
	tcpBalancer  *tcp.Balancer
	udpBalancer  *udp.Balancer
	wg           sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config) *Server {
	return &Server{
		config: cfg,
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 启动HTTP负载均衡器
	if s.config.Balancer.HTTP.Enabled {
		balancer, err := balancerhttp.NewBalancer(s.config.Balancer.HTTP)
		if err != nil {
			return err
		}
		s.httpBalancer = balancer

		httpServer := &http.Server{
			Addr:    ":" + strconv.Itoa(s.config.Balancer.HTTP.Listen),
			Handler: balancer,
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			util.LogInfof("Starting HTTP balancer on port %d", s.config.Balancer.HTTP.Listen)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				util.LogErrorf("HTTP balancer error: %v", err)
			}
		}()
	}

	// 启动TCP负载均衡器
	if s.config.Balancer.TCP.Enabled {
		balancer, err := tcp.NewBalancer(s.config.Balancer.TCP)
		if err != nil {
			return err
		}
		s.tcpBalancer = balancer

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			util.LogInfof("Starting TCP balancer on port %d", s.config.Balancer.TCP.Listen)
			if err := balancer.ListenAndServe(); err != nil {
				util.LogErrorf("TCP balancer error: %v", err)
			}
		}()
	}

	// 启动UDP负载均衡器
	if s.config.Balancer.UDP.Enabled {
		balancer, err := udp.NewBalancer(s.config.Balancer.UDP)
		if err != nil {
			return err
		}
		s.udpBalancer = balancer

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			util.LogInfof("Starting UDP balancer on port %d", s.config.Balancer.UDP.Listen)
			if err := balancer.ListenAndServe(); err != nil {
				util.LogErrorf("UDP balancer error: %v", err)
			}
		}()
	}

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() {
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
