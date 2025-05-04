package round_robin

import (
	"github.com/coffee-realist/balancer/internal/balancer"
	"sync/atomic"
)

type RoundRobinBalancer interface {
	balancer.Balancer
}

type rrBalancer struct {
	servers []string
	idx     uint64
}

// NewRoundRobinBalancer создаёт RoundRobin по списку адресов.
func NewRoundRobinBalancer(servers []string) RoundRobinBalancer {
	return &rrBalancer{servers: append([]string(nil), servers...)}
}

// Next возвращает следующий сервер по кругу.
func (r *rrBalancer) Next() string {
	n := len(r.servers)
	if n == 0 {
		return ""
	}
	i := atomic.AddUint64(&r.idx, 1)
	return r.servers[i%uint64(n)]
}
