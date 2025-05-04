package adapter

import (
	"github.com/coffee-realist/balancer/internal/balancer"
	"github.com/coffee-realist/balancer/internal/balancer/least_conn"
	"github.com/coffee-realist/balancer/internal/balancer/p2c"
	"github.com/coffee-realist/balancer/internal/balancer/round_robin"
	"sync/atomic"
)

// AdaptiveBalancer выбирает стратегию по нагрузке
// и делегирует ConnAware-события на вложенные реализации.
type AdaptiveBalancer struct {
	rr         round_robin.RoundRobinBalancer
	lc         least_conn.LeastConnBalancer
	p2c        p2c.IP2CBalancer
	stopAll    []balancer.Stoppable
	lowThresh  int64
	highThresh int64
	active     int64
}

func NewAdaptiveBalancer(
	rr round_robin.RoundRobinBalancer,
	lc least_conn.LeastConnBalancer,
	p2c p2c.IP2CBalancer,
	low, high int64,
) *AdaptiveBalancer {
	ab := &AdaptiveBalancer{
		rr:         rr,
		lc:         lc,
		p2c:        p2c,
		lowThresh:  low,
		highThresh: high,
	}
	// Собираем те, у кого есть Stop()
	if s, ok := p2c.(balancer.Stoppable); ok {
		ab.stopAll = append(ab.stopAll, s)
	}
	// (RR и LC у нас не имеют фоновых горутин)
	return ab
}

// Next выбирает стратегию и увеличивает общий счётчик active.
func (a *AdaptiveBalancer) Next() string {
	cur := atomic.LoadInt64(&a.active)
	var srv string

	switch {
	case cur < a.lowThresh:
		srv = a.rr.Next()
	case cur < a.highThresh:
		srv = a.p2c.Next()
	default:
		srv = a.lc.Next()
	}

	// общий счётчик in-flight
	atomic.AddInt64(&a.active, 1)
	return srv
}

// Done нужно вызывать, когда запрос завершён: уменьшаем active.
func (a *AdaptiveBalancer) Done() {
	atomic.AddInt64(&a.active, -1)
}

// Increase делегирует только тем стратегиям, которые это поддерживают.
func (a *AdaptiveBalancer) Increase(server string) {
	a.p2c.Increase(server)
	a.lc.Increase(server)
}

// Decrease аналогично
func (a *AdaptiveBalancer) Decrease(server string) {
	a.p2c.Decrease(server)
	a.lc.Decrease(server)
}

// Stop завершает фоновые горутины
func (a *AdaptiveBalancer) Stop() {
	for _, s := range a.stopAll {
		s.Stop()
	}
}
