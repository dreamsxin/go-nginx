package http

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamsxin/go-nginx/config"
	"github.com/dreamsxin/go-nginx/util"
)

// HTTP负载均衡器
type Balancer struct {
	config            config.HTTPBalancerConfig
	backends          []*Backend
	strategy          Strategy
	sessionStore      SessionStore
	mutex             sync.RWMutex
	healthCheckTicker *time.Ticker
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
func NewBalancer(cfg config.HTTPBalancerConfig) (*Balancer, error) {
	b := &Balancer{
		config: cfg,
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
			util.LogErrorf("Proxy error to %s: %v", backendCfg.Address, err)
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
	backend := b.NextBackend(r)
	if backend == nil {
		http.Error(w, "No available backend servers", http.StatusServiceUnavailable)
		return
	}

	// 更新连接数
	atomic.AddInt32(&backend.connections, 1)
	defer atomic.AddInt32(&backend.connections, -1)

	// 会话保持处理
	if b.sessionStore != nil {
		b.sessionStore.SaveBackend(w, r, backend)
	}

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

			// 如果失败次数超过阈值，标记为不可用
			if backend.failCount >= int32(backend.config.MaxFails) {
				backend.alive = false
				util.LogWarnf("Backend %s marked as down after %d failures", address, backend.failCount)
			}
			return
		}
	}
}

// 启动健康检查
func (b *Balancer) startHealthChecks() {
	// 每10秒执行一次健康检查
	b.healthCheckTicker = time.NewTicker(10 * time.Second)
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
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 500 {
				if err != nil {
					util.LogErrorf("Health check failed for %s: %v", backend.config.Address, err)
				} else {
					util.LogErrorf("Health check failed for %s: status code %d", backend.config.Address, resp.StatusCode)
				}
				backend.mutex.Lock()
				backend.alive = false
				backend.failCount++
				backend.mutex.Unlock()
			} else {
				resp.Body.Close()
				backend.mutex.Lock()
				// 如果之前是不可用状态，现在恢复可用
				if !backend.alive {
					util.LogInfof("Backend %s recovered", backend.config.Address)
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
