package least_conn

import (
	"github.com/coffee-realist/balancer/internal/balancer"
	"sync"
	"sync/atomic"
)

type LeastConnBalancer interface {
	balancer.Balancer
	balancer.ConnAware
}

// lcBalancer выбирает сервер с наименьшим числом активных соединений.
type lcBalancer struct {
	servers []string
	counts  map[string]*int64
	mu      sync.RWMutex
}

// NewLeastConnBalancer создаёт Least-Conn балансировщик.
func NewLeastConnBalancer(servers []string) LeastConnBalancer {
	counts := make(map[string]*int64, len(servers))
	for _, s := range servers {
		var zero int64
		counts[s] = &zero
	}
	return &lcBalancer{
		servers: append([]string(nil), servers...),
		counts:  counts,
	}
}

// Next возвращает сервер с минимальным числом активных соединений.
func (l *lcBalancer) Next() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var best string
	var mn int64 = -1
	for _, s := range l.servers {
		cnt := atomic.LoadInt64(l.counts[s])
		if mn < 0 || cnt < mn {
			mn = cnt
			best = s
		}
	}
	return best
}

// Increase увеличивает счётчик для conn-aware стратегий.
func (l *lcBalancer) Increase(server string) {
	if ptr, ok := l.counts[server]; ok {
		atomic.AddInt64(ptr, 1)
	}
}

// Decrease уменьшает счётчик.
func (l *lcBalancer) Decrease(server string) {
	if ptr, ok := l.counts[server]; ok {
		atomic.AddInt64(ptr, -1)
	}
}
