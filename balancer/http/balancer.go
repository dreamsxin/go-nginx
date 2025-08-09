package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/monitor"
)

// HTTP负载均衡器
type Balancer struct {
	config            config.HTTPBalancerConfig
	backends          []*Backend
	strategy          Strategy
	sessionStore      SessionStore
	mutex             sync.RWMutex
	healthCheckTicker *time.Ticker
	metrics           *monitor.Metrics
	statsManager      *monitor.StatsManager
}

// 后端服务器
type Backend struct {
	url          *url.URL
	proxy        *httputil.ReverseProxy
	config       config.BackendConfig
	alive        bool
	connections  int32
	failCount    int32
	lastFailTime time.Time
	mutex        sync.RWMutex
}

// 新建HTTP负载均衡器
func NewBalancer(cfg config.HTTPBalancerConfig, metrics *monitor.Metrics, statsManager *monitor.StatsManager) (*Balancer, error) {
	b := &Balancer{
		config:       cfg,
		metrics:      metrics,
		statsManager: statsManager,
	}

	// 初始化后端服务器
	for _, backendCfg := range cfg.Backends {
		url, err := url.Parse("http://" + backendCfg.Address)
		if err != nil {
			return nil, err
		}

		proxy := httputil.NewSingleHostReverseProxy(url)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			// 添加X-Forwarded-For头
			req.Header.Set("X-Forwarded-For", req.RemoteAddr)
			req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
		}

		// 自定义错误处理
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error(fmt.Sprintf("Proxy error to %s: %v", backendCfg.Address, err))
			b.markBackendFailed(backendCfg.Address)
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		}

		b.backends = append(b.backends, &Backend{
			url:    url,
			proxy:  proxy,
			config: backendCfg,
			alive:  true,
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

	// 初始化会话保持
	if cfg.SessionPersistence.Enabled {
		b.sessionStore = NewCookieSessionStore(cfg.SessionPersistence.CookieName, cfg.SessionPersistence.CookieMaxAge)
	}

	// 启动健康检查
	b.startHealthChecks()

	return b, nil
}

// 选择后端服务器
func (b *Balancer) NextBackend(r *http.Request) *Backend {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// 会话保持优先
	if b.sessionStore != nil {
		if backend := b.sessionStore.GetBackend(r); backend != nil && backend.alive {
			return backend
		}
	}

	// 使用策略选择后端
	return b.strategy.Next(r)
}

// 处理HTTP请求
func (b *Balancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		b.metrics.RecordRequest(duration, false)
	}()
	backend := b.NextBackend(r)
	if backend == nil {
		http.Error(w, "No available backend servers", http.StatusServiceUnavailable)
		return
	}
	slog.Info(fmt.Sprintf("Proxying request to %s", backend.config.Address))

	// 更新连接数
	atomic.AddInt32(&backend.connections, 1)
	defer atomic.AddInt32(&backend.connections, -1)

	// 会话保持处理
	if b.sessionStore != nil {
		b.sessionStore.SaveBackend(w, r, backend)
	}

	b.statsManager.UpdateBackendRequests(backend.url.String(), false)
	b.statsManager.UpdateConnectionStats(backend.url.String(), 1)
	defer b.statsManager.UpdateConnectionStats(backend.url.String(), -1)

	// 转发请求
	backend.proxy.ServeHTTP(w, r)
}

// 标记后端服务器为失败
func (b *Balancer) markBackendFailed(address string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, backend := range b.backends {
		if backend.url.Host == address {
			backend.failCount++
			backend.lastFailTime = time.Now()

			b.statsManager.UpdateBackendFailures(address)

			// 如果失败次数超过阈值，标记为不可用
			if backend.failCount >= int32(backend.config.MaxFails) {
				backend.alive = false
				slog.Warn(fmt.Sprintf("Backend %s marked as down after %d failures", address, backend.failCount))
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
	// 每10秒执行一次健康检查
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
		// 如果服务器已标记为不可用且未超过失败超时时间，则跳过检查
		if !backend.alive && time.Since(backend.lastFailTime) < backend.config.FailTimeout {
			continue
		}

		// 执行HTTP HEAD请求检查
		go func(backend *Backend) {
			client := http.Client{
				Timeout: 5 * time.Second,
			}

			resp, err := client.Head("http://" + backend.config.Address)
			if err != nil {
				slog.Error(fmt.Sprintf("Health check failed for %s: %v", backend.config.Address, err))
				backend.mutex.Lock()
				backend.alive = false
				backend.failCount++
				backend.lastFailTime = time.Now()
				backend.mutex.Unlock()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 500 {

				slog.Error(fmt.Sprintf("Health check failed for %s: status code %d", backend.config.Address, resp.StatusCode))
				backend.mutex.Lock()
				backend.alive = false
				backend.failCount++
				backend.lastFailTime = time.Now()
				backend.mutex.Unlock()
			} else {
				backend.mutex.Lock()
				// 如果之前是不可用状态，现在恢复可用
				if !backend.alive {
					slog.Info(fmt.Sprintf("Backend %s recovered", backend.config.Address))
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
}

// UpdateConfig 更新负载均衡器配置
func (b *Balancer) UpdateConfig(newConfig config.HTTPBalancerConfig) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// 更新后端服务器
	for _, backendCfg := range newConfig.Backends {
		// 检查是否已存在该后端
		existing := false
		for _, oldBackend := range b.backends {
			if oldBackend.config.Address == backendCfg.Address {
				existing = true
				break
			}
		}
		if !existing {

			url, err := url.Parse("http://" + backendCfg.Address)
			if err != nil {
				slog.Error(fmt.Sprintf("Invalid backend address: %s", backendCfg.Address))
				continue
			}

			proxy := httputil.NewSingleHostReverseProxy(url)
			originalDirector := proxy.Director
			proxy.Director = func(req *http.Request) {
				originalDirector(req)
				// 添加X-Forwarded-For头
				req.Header.Set("X-Forwarded-For", req.RemoteAddr)
				req.Header.Set("X-Forwarded-Proto", req.URL.Scheme)
			}

			// 自定义错误处理
			proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Error(fmt.Sprintf("Proxy error to %s: %v", backendCfg.Address, err))
				b.markBackendFailed(backendCfg.Address)
				http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			}

			b.backends = append(b.backends, &Backend{
				url:    url,
				proxy:  proxy,
				config: backendCfg,
				alive:  true,
			})
		}
	}

	return nil
}
