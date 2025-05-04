package least_conn

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupLC — вспомогательная функция для создания LeastConnBalancer с заданными серверами.
func setupLC(servers []string) LeastConnBalancer {
	return NewLeastConnBalancer(servers)
}

func TestLeastConnBalancer_InitialAndBasic(t *testing.T) {
	servers := []string{"X", "Y", "Z"}
	lc := setupLC(servers)

	// Тест 1: Изначально выбирается первый сервер (все счетчики равны нулю)
	t.Run("initially picks first (all zero)", func(t *testing.T) {
		got := lc.Next()
		assert.Equal(t, "X", got)
	})

	// Тест 2: После увеличения для X выбирается следующий сервер с минимальной нагрузкой
	t.Run("after increase on X picks next-min", func(t *testing.T) {
		lc.Increase("X")
		got := lc.Next()
		assert.Equal(t, "Y", got)
	})

	// Тест 3: Проверка работы уменьшения нагрузки на сервер
	t.Run("decrease works", func(t *testing.T) {
		lc.Decrease("X")
		got := lc.Next()
		assert.Equal(t, "X", got)
	})
}

func TestLeastConnBalancer_MultipleIncreases(t *testing.T) {
	servers := []string{"A", "B"}
	lc := setupLC(servers)

	lc.Increase("A")
	lc.Increase("A")
	lc.Increase("B")

	// Ожидаем, что балансировщик будет всегда выбирать сервер с меньшей нагрузкой (в данном случае "B")
	for i := 0; i < 3; i++ {
		got := lc.Next()
		assert.Equal(t, "B", got)
	}
}

// TestLeastConnBalancer_Concurrency тестирует многозадачность с несколькими горутинами
func TestLeastConnBalancer_Concurrency(t *testing.T) {
	servers := []string{"foo", "bar", "baz"}
	lc := setupLC(servers)

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()
			lc.Increase(srv)
			lc.Decrease(srv)
		}(servers[i%3])
	}
	wg.Wait()

	// Проверяем, что после всех операций нагрузка на все серверы вернулась в исходное состояние
	assert.Equal(t, int64(0), atomic.LoadInt64(lc.(*lcBalancer).counts["foo"]))
	assert.Equal(t, int64(0), atomic.LoadInt64(lc.(*lcBalancer).counts["bar"]))
	assert.Equal(t, int64(0), atomic.LoadInt64(lc.(*lcBalancer).counts["baz"]))
}

// BenchmarkLeastConnBalancer_Next тестирует производительность метода Next при большом количестве запросов.
func BenchmarkLeastConnBalancer_Next(b *testing.B) {
	servers := []string{"one", "two", "three", "four", "five"}
	lc := setupLC(servers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lc.Next()
	}
}

// BenchmarkLeastConnBalancer_IncreaseDecrease тестирует производительность операций Increase и Decrease.
func BenchmarkLeastConnBalancer_IncreaseDecrease(b *testing.B) {
	servers := []string{"one", "two", "three"}
	lc := setupLC(servers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Increase("one")
		lc.Decrease("one")
	}
}
