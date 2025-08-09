package udp

import (
	"hash/fnv"
	"net"
	"sync/atomic"
)

// 负载均衡策略接口
type Strategy interface {
	Next(clientIP string) *Backend
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
func (s *RoundRobinStrategy) Next(clientIP string) *Backend {
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

// 根据客户端IP选择后端
func (s *IPHashStrategy) Next(clientIP string) *Backend {
	backends := s.balancer.backends
	if len(backends) == 0 {
		return nil
	}

	// 提取客户端IP
	ip, _, err := net.SplitHostPort(clientIP)
	if err != nil {
		ip = clientIP
	}

	// 计算IP哈希
	hash := fnv.New32a()
	hash.Write([]byte(ip))
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
