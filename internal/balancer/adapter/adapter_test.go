package adapter

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubRR — фиктивная реализация для RoundRobinBalancer.
type stubRR struct{ srv string }

func (s *stubRR) Next() string { return s.srv }

// stubLC — фиктивная реализация для LeastConnBalancer, поддерживающая операции Increase и Decrease.
type stubLC struct {
	srv string
	cnt int64
}

func (s *stubLC) Next() string      { return s.srv }
func (s *stubLC) Increase(_ string) { atomic.AddInt64(&s.cnt, 1) }
func (s *stubLC) Decrease(_ string) { atomic.AddInt64(&s.cnt, -1) }
func (s *stubLC) Stop()             {}

type stubP2C = stubLC // Используется как псевдоним для stubLC.

func TestAdaptiveBalancer_SwitchingAndConnAware(t *testing.T) {
	rr := &stubRR{srv: "rr"}
	lc := &stubLC{srv: "lc"}
	p2c := &stubP2C{srv: "p2c"}

	ab := NewAdaptiveBalancer(rr, lc, p2c, 5, 10)

	// Тест 1: низкая нагрузка → использует RoundRobinBalancer (rr)
	atomic.StoreInt64(&ab.active, 0)
	srv := ab.Next()
	assert.Equal(t, "rr", srv)
	ab.Increase(srv)
	ab.Decrease(srv)
	assert.Equal(t, int64(0), atomic.LoadInt64(&lc.cnt))
	assert.Equal(t, int64(0), atomic.LoadInt64(&p2c.cnt))
	ab.Done()

	// Тест 2: средняя нагрузка → использует P2CBalancer (p2c)
	atomic.StoreInt64(&ab.active, 6)
	srv = ab.Next()
	assert.Equal(t, "p2c", srv)
	ab.Increase(srv)
	assert.Equal(t, int64(1), atomic.LoadInt64(&p2c.cnt))
	ab.Decrease(srv)
	ab.Done()

	// Тест 3: высокая нагрузка → использует LeastConnBalancer (lc)
	atomic.StoreInt64(&ab.active, 12)
	srv = ab.Next()
	assert.Equal(t, "lc", srv)
	ab.Increase(srv)
	assert.Equal(t, int64(1), atomic.LoadInt64(&lc.cnt))
	ab.Decrease(srv)
	ab.Done()
}

// BenchmarkAdaptiveBalancer_Next проверяет производительность метода Next при высокой нагрузке.
func BenchmarkAdaptiveBalancer_Next(b *testing.B) {
	rr := &stubRR{srv: "rr"}
	lc := &stubLC{srv: "lc"}
	p2c := &stubP2C{srv: "p2c"}
	ab := NewAdaptiveBalancer(rr, lc, p2c, 1000, 2000)
	defer ab.Stop()

	// Настройка высокой нагрузки для использования LeastConnBalancer (lc)
	atomic.StoreInt64(&ab.active, 3000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ab.Next()
	}
}
