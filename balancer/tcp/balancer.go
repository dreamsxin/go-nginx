package tcp

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-nginx/config"
)

// TCP负载均衡器
type Balancer struct {
	config            config.TCPBalancerConfig
	backends          []*Backend
	strategy          Strategy
	mutex             sync.RWMutex
	healthCheckTicker *time.Ticker
	listener          net.Listener
}

// 后端服务器
type Backend struct {
	address      string
	config       config.BackendConfig
	alive        bool
	connections  int32
	failCount    int32
	lastFailTime time.Time
	mutex        sync.RWMutex
}

// 新建TCP负载均衡器
func NewBalancer(cfg config.TCPBalancerConfig) (*Balancer, error) {
	b := &Balancer{
		config: cfg,
	}

	// 初始化后端服务器
	for _, backendCfg := range cfg.Backends {
		b.backends = append(b.backends, &Backend{
			address: backendCfg.Address,
			config:  backendCfg,
			alive:   true,
		})
	}

	// 初始化负载均衡策略
	switch cfg.Strategy {
	case "round_robin":
		b.strategy = NewRoundRobinStrategy(b)
	case "least_conn":
		b.strategy = NewLeastConnStrategy(b)
	case "ip_hash":
		b.strategy = NewIPHashStrategy(b)
	default:
		b.strategy = NewRoundRobinStrategy(b)
	}

	// 启动健康检查
	if cfg.HealthCheck.Enabled {
		b.startHealthChecks()
	}

	return b, nil
}

// 启动TCP监听器
func (b *Balancer) ListenAndServe() error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(b.config.Listen))
	if err != nil {
		return err
	}
	defer listener.Close()
	b.listener = listener

	slog.Info(fmt.Sprintf("TCP balancer listening on port %d", b.config.Listen))

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Error(fmt.Sprintf("Accept error: %v", err))
			// 判断错误类型
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				slog.Error(fmt.Sprintf("Accept timeout error: %v", err))
				continue
			} else {
				slog.Error(fmt.Sprintf("Accept error: %v", err))
			}
			return err
		}

		backend := b.NextBackend(conn.RemoteAddr().String())
		if backend == nil {
			conn.Close()
			continue
		}

		// 转发连接
		go b.handleConnection(conn, backend)
	}
}

// 处理TCP连接
func (b *Balancer) handleConnection(clientConn net.Conn, backend *Backend) {
	defer clientConn.Close()

	// 设置客户端连接超时
	clientConn.SetDeadline(time.Now().Add(30 * time.Second))

	// 增加连接计数
	atomic.AddInt32(&backend.connections, 1)
	defer atomic.AddInt32(&backend.connections, -1)

	// 连接后端服务器，设置超时
	backendConn, err := net.DialTimeout("tcp", backend.address, 5*time.Second)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to connect to backend %s: %v", backend.address, err))
		b.markBackendFailed(backend.address)
		return
	}
	defer backendConn.Close()

	// 设置后端连接超时
	backendConn.SetDeadline(time.Now().Add(30 * time.Second))

	// 双向转发数据
	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			// 重置超时
			clientConn.SetDeadline(time.Now().Add(30 * time.Second))
			n, err := clientConn.Read(buf)
			if n > 0 {
				if _, err := backendConn.Write(buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			// 重置超时
			backendConn.SetDeadline(time.Now().Add(30 * time.Second))
			n, err := backendConn.Read(buf)
			if n > 0 {
				if _, err := clientConn.Write(buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	<-done
}

// 选择后端服务器
func (b *Balancer) NextBackend(clientIP string) *Backend {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.strategy.Next(clientIP)
}

// 标记后端服务器为失败
func (b *Balancer) markBackendFailed(address string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, backend := range b.backends {
		if backend.address == address {
			backend.failCount++
			backend.lastFailTime = time.Now()

			if backend.failCount >= int32(backend.config.MaxFails) {
				backend.alive = false
				slog.Warn(fmt.Sprintf("TCP backend %s marked as down after %d failures", address, backend.failCount))
			}
			return
		}
	}
}

// 启动健康检查
func (b *Balancer) startHealthChecks() {
	// 使用配置的检查间隔，默认为10秒
	interval := b.config.HealthCheck.Interval
	if interval == 0 {
		interval = 10 * time.Second
	}
	b.healthCheckTicker = time.NewTicker(interval)
	go func() {
		for range b.healthCheckTicker.C {
			b.runHealthChecks()
		}
	}()
}

// 执行健康检查
func (b *Balancer) runHealthChecks() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, backend := range b.backends {
		if !backend.alive && time.Since(backend.lastFailTime) < backend.config.FailTimeout {
			continue
		}

		go func(backend *Backend) {
			// 使用配置的超时值，默认为5秒
			timeout := b.config.HealthCheck.Timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}
			conn, err := net.DialTimeout("tcp", backend.address, timeout)
			if err != nil {
				slog.Error(fmt.Sprintf("TCP health check failed for %s: %v", backend.address, err))
				backend.mutex.Lock()
				backend.alive = false
				backend.failCount++
				backend.mutex.Unlock()
			} else {
				conn.Close()
				backend.mutex.Lock()
				if !backend.alive {
					slog.Info(fmt.Sprintf("TCP backend %s recovered", backend.address))
					backend.alive = true
					backend.failCount = 0
				}
				backend.mutex.Unlock()
			}
		}(backend)
	}
}

// 停止负载均衡器
func (b *Balancer) Stop() {
	if b.healthCheckTicker != nil {
		b.healthCheckTicker.Stop()
	}
	if b.listener != nil {
		b.listener.Close()
	}
}
