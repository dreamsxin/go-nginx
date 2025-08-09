package udp

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

// UDP负载均衡器
type Balancer struct {
	config            config.UDPBalancerConfig
	backends          []*Backend
	strategy          Strategy
	mutex             sync.RWMutex
	healthCheckTicker *time.Ticker
	conn              *net.UDPConn
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

// 新建UDP负载均衡器
func NewBalancer(cfg config.UDPBalancerConfig) (*Balancer, error) {
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
	case "ip_hash":
		b.strategy = NewIPHashStrategy(b)
	default:
		b.strategy = NewRoundRobinStrategy(b)
	}

	// 启动健康检查
	b.startHealthChecks()

	return b, nil
}

// 启动UDP监听器
func (b *Balancer) ListenAndServe() error {
	addr, err := net.ResolveUDPAddr("udp", ":"+strconv.Itoa(b.config.Listen))
	if err != nil {
		return err
	}

	b.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer b.conn.Close()

	slog.Info(fmt.Sprintf("UDP balancer listening on port %d", b.config.Listen))

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := b.conn.ReadFromUDP(buf)
		if err != nil {
			slog.Error(fmt.Sprintf("UDP read error: %v", err))
			return err
		}

		backend := b.NextBackend(clientAddr.String())
		if backend == nil {
			continue
		}

		// 转发数据
		go b.handlePacket(clientAddr, backend, buf[:n])
	}
}

// 处理UDP数据包
func (b *Balancer) handlePacket(clientAddr *net.UDPAddr, backend *Backend, data []byte) {
	// 增加连接计数
	atomic.AddInt32(&backend.connections, 1)
	defer atomic.AddInt32(&backend.connections, -1)

	backendAddr, err := net.ResolveUDPAddr("udp", backend.address)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to resolve backend address %s: %v", backend.address, err))
		b.markBackendFailed(backend.address)
		return
	}

	// 设置写超时
	b.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	// 发送到后端
	n, err := b.conn.WriteToUDP(data, backendAddr)
	if err != nil || n != len(data) {
		slog.Error(fmt.Sprintf("Failed to send UDP packet to %s: %v", backend.address, err))
		b.markBackendFailed(backend.address)
		return
	}
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
			backend.mutex.Lock()
			backend.failCount++
			backend.lastFailTime = time.Now()

			// 如果失败次数超过阈值，标记为不可用
			if backend.failCount >= int32(backend.config.MaxFails) {
				backend.alive = false
				slog.Warn(fmt.Sprintf("Backend %s marked as down after %d failures", address, backend.failCount))
			}
			backend.mutex.Unlock()
			return
		}
	}
}

// 启动健康检查
func (b *Balancer) startHealthChecks() {
	// 从配置获取健康检查间隔，默认为10秒
	interval := 10 * time.Second
	if b.config.HealthCheck.Interval > 0 {
		interval = b.config.HealthCheck.Interval
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
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, backend := range b.backends {
		go func(backend *Backend) {
			backend.mutex.Lock()
			defer backend.mutex.Unlock()

			// 如果服务器已标记为不可用且未超过失败超时时间，则跳过检查
			if !backend.alive && time.Since(backend.lastFailTime) < backend.config.FailTimeout {
				return
			}

			// UDP健康检查：发送空数据包并等待响应
			conn, err := net.DialTimeout("udp", backend.address, 2*time.Second)
			if err != nil {
				slog.Error(fmt.Sprintf("Health check failed for %s: %v", backend.address, err))
				backend.alive = false
				backend.failCount++
				return
			}
			defer conn.Close()

			// 发送健康检查数据包
			healthCheckData := []byte("healthcheck")
			_, err = conn.Write(healthCheckData)
			if err != nil {
				slog.Error(fmt.Sprintf("Health check failed for %s: %v", backend.address, err))
				backend.alive = false
				backend.failCount++
				return
			}

			// 设置读取超时
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))

			// 读取响应
			buf := make([]byte, 1024)
			_, err = conn.Read(buf)
			if err != nil {
				slog.Error(fmt.Sprintf("Health check failed for %s: %v", backend.address, err))
				backend.alive = false
				backend.failCount++
				return
			}

			// 如果之前是不可用状态，现在恢复可用
			if !backend.alive {
				slog.Info(fmt.Sprintf("Backend %s recovered", backend.address))
				backend.alive = true
				backend.failCount = 0
			}
		}(backend)
	}
}

// 停止负载均衡器
func (b *Balancer) Stop() {
	if b.healthCheckTicker != nil {
		b.healthCheckTicker.Stop()
	}
	if b.conn != nil {
		b.conn.Close()
	}
}
