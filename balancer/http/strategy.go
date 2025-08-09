package http

import (
	"hash/fnv"
	"net"
	"net/http"
	"sync/atomic"
)

// 负载均衡策略接口
type Strategy interface {
	Next(r *http.Request) *Backend
}

// 轮询策略
type RoundRobinStrategy struct {
	balancer *Balancer
	current  uint32
}

// 新建轮询策略
func NewRoundRobinStrategy(balancer *Balancer) *RoundRobinStrategy {
	return &RoundRobinStrategy{
		balancer: balancer,
	}
}

// 选择下一个后端
func (s *RoundRobinStrategy) Next(r *http.Request) *Backend {
	backends := s.balancer.backends
	if len(backends) == 0 {
		return nil
	}

	// 原子递增当前索引
	current := atomic.AddUint32(&s.current, 1) - 1
	index := current % uint32(len(backends))

	// 查找可用后端
	for i := 0; i < len(backends); i++ {
		backend := backends[(index+uint32(i))%uint32(len(backends))]
		backend.mutex.RLock()
		alive := backend.alive
		backend.mutex.RUnlock()
		if alive {
			return backend
		}
	}

	return nil
}

// 最小连接数策略
type LeastConnStrategy struct {
	balancer *Balancer
}

// 新建最小连接数策略
func NewLeastConnStrategy(balancer *Balancer) *LeastConnStrategy {
	return &LeastConnStrategy{
		balancer: balancer,
	}
}

// 选择连接数最少的后端
func (s *LeastConnStrategy) Next(r *http.Request) *Backend {
	backends := s.balancer.backends
	if len(backends) == 0 {
		return nil
	}

	var leastBackend *Backend
	var minConnections int32 = -1

	for _, backend := range backends {
		backend.mutex.RLock()
		alive := backend.alive
		connections := backend.connections
		backend.mutex.RUnlock()

		if alive && (minConnections == -1 || connections < minConnections) {
			minConnections = connections
			leastBackend = backend
		}
	}

	return leastBackend
}

// IP哈希策略
type IPHashStrategy struct {
	balancer *Balancer
}

// 新建IP哈希策略
func NewIPHashStrategy(balancer *Balancer) *IPHashStrategy {
	return &IPHashStrategy{
		balancer: balancer,
	}
}

// 基于客户端IP选择后端
func (s *IPHashStrategy) Next(r *http.Request) *Backend {
	backends := s.balancer.backends
	if len(backends) == 0 {
		return nil
	}

	// 获取客户端IP
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}

	// 计算IP哈希
	hash := fnv.New32a()
	hash.Write([]byte(clientIP))
	index := hash.Sum32() % uint32(len(backends))

	// 查找可用后端
	for i := 0; i < len(backends); i++ {
		backend := backends[(index+uint32(i))%uint32(len(backends))]
		backend.mutex.RLock()
		alive := backend.alive
		backend.mutex.RUnlock()
		if alive {
			return backend
		}
	}

	return nil
}

// 带权重的轮询策略
type WeightedRoundRobinStrategy struct {
	balancer  *Balancer
	current   int
	weightSum int
}

// 新建带权重的轮询策略
func NewWeightedRoundRobinStrategy(balancer *Balancer) *WeightedRoundRobinStrategy {
	// 计算权重总和
	weightSum := 0
	for _, backend := range balancer.backends {
		weightSum += backend.config.Weight
	}

	return &WeightedRoundRobinStrategy{
		balancer:  balancer,
		weightSum: weightSum,
		current:   -1,
	}
}

// 选择下一个后端（带权重）
func (s *WeightedRoundRobinStrategy) Next(r *http.Request) *Backend {
	backends := s.balancer.backends
	if len(backends) == 0 {
		return nil
	}

	// 如果权重总和为0，使用普通轮询
	if s.weightSum == 0 {
		strategy := NewRoundRobinStrategy(s.balancer)
		return strategy.Next(r)
	}

	// 查找可用后端并计算可用权重总和
	availableBackends := []*Backend{}
	availableWeightSum := 0
	for _, backend := range backends {
		backend.mutex.RLock()
		alive := backend.alive
		backend.mutex.RUnlock()

		if alive {
			availableBackends = append(availableBackends, backend)
			availableWeightSum += backend.config.Weight
		}
	}

	if len(availableBackends) == 0 {
		return nil
	}

	// 使用加权轮询算法选择后端
	s.current = (s.current + 1) % availableWeightSum
	currentWeight := s.current

	for _, backend := range availableBackends {
		currentWeight -= backend.config.Weight
		if currentWeight < 0 {
			return backend
		}
	}

	// 作为后备，返回第一个可用后端
	return availableBackends[0]
}
