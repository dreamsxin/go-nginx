package monitor

import (
	"sync"
	"time"
)

// BackendStats 后端服务器统计信息
type BackendStats struct {
	URL        string
	Healthy    bool
	LastCheck  time.Time
	Requests   int64
	Errors     int64
	ActiveConn int
	MaxConn    int
}

// StatsManager 统计数据管理器
type StatsManager struct {
	backendStats map[string]*BackendStats
	mutex        sync.RWMutex
}

// NewStatsManager 创建新的统计数据管理器
func NewStatsManager() *StatsManager {
	return &StatsManager{
		backendStats: make(map[string]*BackendStats),
	}
}

// UpdateBackendStatus 更新后端服务器状态
func (s *StatsManager) UpdateBackendStatus(backendURL string, healthy bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats, exists := s.backendStats[backendURL]
	if !exists {
		stats = &BackendStats{URL: backendURL}
		s.backendStats[backendURL] = stats
	}

	stats.Healthy = healthy
	stats.LastCheck = time.Now()
}

// UpdateBackendRequests 更新后端服务器请求统计
func (s *StatsManager) UpdateBackendRequests(backendURL string, isError bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats, exists := s.backendStats[backendURL]
	if !exists {
		stats = &BackendStats{URL: backendURL}
		s.backendStats[backendURL] = stats
	}

	stats.Requests++
	if isError {
		stats.Errors++
	}
}

func (s *StatsManager) UpdateBackendFailures(backendURL string) {

	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats, exists := s.backendStats[backendURL]
	if !exists {
		stats = &BackendStats{URL: backendURL}
		s.backendStats[backendURL] = stats
	}

	stats.Errors++
}

// GetAllBackendStats 获取所有后端服务器统计信息
func (s *StatsManager) GetAllBackendStats() []*BackendStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := make([]*BackendStats, 0, len(s.backendStats))
	for _, s := range s.backendStats {
		stats = append(stats, s)
	}
	return stats
}

// UpdateConnectionStats 更新连接统计
func (s *StatsManager) UpdateConnectionStats(backendURL string, delta int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats, exists := s.backendStats[backendURL]
	if !exists {
		stats = &BackendStats{URL: backendURL}
		s.backendStats[backendURL] = stats
	}

	stats.ActiveConn += delta
	if stats.ActiveConn < 0 {
		stats.ActiveConn = 0
	}
}
