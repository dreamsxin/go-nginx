package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// 会话存储接口
type SessionStore interface {
	GetBackend(r *http.Request) *Backend
	SaveBackend(w http.ResponseWriter, r *http.Request, backend *Backend)
}

// 会话数据结构
type SessionData struct {
	Backend   *Backend
	ExpiresAt time.Time
}

// Cookie会话存储
type CookieSessionStore struct {
	cookieName string
	sessions   map[string]SessionData
	mu         sync.RWMutex
	expiryTime time.Duration
}

// 创建Cookie会话存储
func NewCookieSessionStore(cookieName string, expiryTime time.Duration) *CookieSessionStore {
	if cookieName == "" {
		cookieName = "GO_NGINX_SESSION"
	}
	if expiryTime <= 0 {
		expiryTime = 24 * time.Hour
	}

	store := &CookieSessionStore{
		cookieName: cookieName,
		sessions:   make(map[string]SessionData),
		expiryTime: expiryTime,
	}

	// 启动会话清理goroutine
	go store.cleanupExpiredSessions()

	return store
}

// 清理过期会话
func (s *CookieSessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, data := range s.sessions {
			if data.ExpiresAt.Before(now) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// 从请求中获取后端服务器
func (s *CookieSessionStore) GetBackend(r *http.Request) *Backend {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return nil
	}

	sessionID := cookie.Value
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.sessions[sessionID]
	if !exists || data.ExpiresAt.Before(time.Now()) {
		return nil
	}

	// 更新会话过期时间（滑动窗口）
	if s.expiryTime > 0 {
		s.mu.RUnlock()
		s.mu.Lock()
		data.ExpiresAt = time.Now().Add(s.expiryTime)
		s.sessions[sessionID] = data
		s.mu.Unlock()
		s.mu.RLock()
	}

	return data.Backend
}

// 保存会话信息到响应
func (s *CookieSessionStore) SaveBackend(w http.ResponseWriter, r *http.Request, backend *Backend) {
	// 检查请求中是否已有会话cookie
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		// 生成新的session ID
		sessionID := uuid.New().String()

		s.mu.Lock()
		s.sessions[sessionID] = SessionData{
			Backend:   backend,
			ExpiresAt: time.Now().Add(s.expiryTime),
		}
		s.mu.Unlock()

		// 设置cookie
		cookie := http.Cookie{
			Name:     s.cookieName,
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			MaxAge:   int(s.expiryTime.Seconds()),
		}
		http.SetCookie(w, &cookie)
		return
	}

	// 更新现有会话
	sessionID := cookie.Value
	s.mu.Lock()
	defer s.mu.Unlock()

	data, exists := s.sessions[sessionID]
	if !exists {
		data = SessionData{}
	}

	data.Backend = backend
	data.ExpiresAt = time.Now().Add(s.expiryTime)
	s.sessions[sessionID] = data

	// 更新cookie过期时间
	cookie.MaxAge = int(s.expiryTime.Seconds())
	http.SetCookie(w, cookie)
}
