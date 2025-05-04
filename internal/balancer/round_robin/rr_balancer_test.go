package round_robin

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRoundRobinBalancer_Basic проверяет базовую корректность работы алгоритма Round Robin.
func TestRoundRobinBalancer_Basic(t *testing.T) {
	servers := []string{"A", "B", "C"}
	rr := NewRoundRobinBalancer(servers)
	n := len(servers)

	t.Run("cycle through servers", func(t *testing.T) {
		// Проверка циклического перебора серверов
		for cycle := 0; cycle < 2; cycle++ {
			for j := 0; j < n; j++ {
				got := rr.Next()
				want := servers[(j+1)%n] // Ожидаемый следующий сервер
				assert.Equal(t, want, got, "iteration %d of cycle %d", j, cycle)
			}
		}
	})

	t.Run("empty list returns empty string", func(t *testing.T) {
		// Проверка поведения при пустом списке серверов
		empty := NewRoundRobinBalancer(nil)
		assert.Equal(t, "", empty.Next())
	})
}

// BenchmarkRoundRobinBalancer_Next измеряет производительность метода Next при 100 серверах.
func BenchmarkRoundRobinBalancer_Next(b *testing.B) {
	servers := make([]string, 100)
	for i := 0; i < 100; i++ {
		servers[i] = "s" + strconv.Itoa(i)
	}
	rr := NewRoundRobinBalancer(servers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rr.Next() // Вызов метода балансировщика в бенчмарке
	}
}
