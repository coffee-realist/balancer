package p2c

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/assert"
)

// setupHealthBalancer создает p2cBalancer с двумя тестовыми серверами и возвращает функцию очистки.
func setupHealthBalancer(code1, code2 int, hcInterval time.Duration) (*p2cBalancer, func()) {
	// Создаем два тестовых HTTP сервера для имитации работы health check.
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(code1)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(code2)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Создаем балансировщик p2c и возвращаем его вместе с функцией очистки.
	b := NewP2CBalancer([]string{s1.URL, s2.URL}, hcInterval).(*p2cBalancer)
	return b, func() {
		// Закрываем сервера и останавливаем балансировщик при завершении теста.
		s1.Close()
		s2.Close()
		b.Stop()
	}
}

// TestP2CBasicDistribution проверяет, что Next возвращает валидный сервер и примерно равномерно распределяет запросы.
func TestP2CBasicDistribution(t *testing.T) {
	// Инициализация фейковых данных с фиксированным seed для тестирования.
	gofakeit.Seed(42)
	servers := []string{"A", "B", "C", "D"}
	b := NewP2CBalancer(servers, 10*time.Second)

	t.Run("ValidServerSelection", func(t *testing.T) {
		// Проверяем, что Next возвращает валидный сервер.
		for i := 0; i < 1000; i++ {
			s := b.Next()
			assert.Contains(t, servers, s, "Next() returned invalid server")
			b.Increase(s)
			b.Decrease(s)
		}
	})

	t.Run("DistributionUniformity", func(t *testing.T) {
		// Проверяем равномерность распределения.
		counts := make(map[string]int)
		for i := 0; i < 10000; i++ {
			s := b.Next()
			counts[s]++
			b.Increase(s)
			b.Decrease(s)
		}
		avg := 10000 / len(servers)
		tolerance := avg / 4
		// Убедимся, что распределение серверов близко к равномерному.
		for _, srv := range servers {
			diff := counts[srv] - avg
			assert.LessOrEqual(t, diff, tolerance, "server %s too high", srv)
			assert.GreaterOrEqual(t, diff, -tolerance, "server %s too low", srv)
			assert.Greater(t, counts[srv], 0, "server %s never selected", srv)
		}
	})
}

// TestHealthChecks проверяет, что Health Checks корректно помечают серверы как up/down.
func TestHealthChecks(t *testing.T) {
	t.Run("InitialAllUp", func(t *testing.T) {
		// Проверяем, что оба сервера доступны в начале.
		b, teardown := setupHealthBalancer(http.StatusOK, http.StatusOK, 50*time.Millisecond)
		defer teardown()
		healthy := b.getHealthyServers()
		assert.Len(t, healthy, 2, "both servers should be up initially")
	})

	t.Run("ExcludeDownAfterTick", func(t *testing.T) {
		// Проверяем, что сервер с ошибкой исключается после одного цикла.
		b, teardown := setupHealthBalancer(http.StatusInternalServerError, http.StatusOK, 50*time.Millisecond)
		defer teardown()
		time.Sleep(75 * time.Millisecond)
		healthy := b.getHealthyServers()
		assert.Len(t, healthy, 1, "exactly one server should be up")
	})

	t.Run("AllDown", func(t *testing.T) {
		// Проверяем, что все серверы исключаются, если они недоступны.
		b, teardown := setupHealthBalancer(http.StatusInternalServerError, http.StatusInternalServerError, 50*time.Millisecond)
		defer teardown()
		time.Sleep(75 * time.Millisecond)
		healthy := b.getHealthyServers()
		assert.Empty(t, healthy, "no servers should be healthy")
	})
}

// TestNextSkipsDownServers убеждается, что Next не возвращает недоступные серверы.
func TestNextSkipsDownServers(t *testing.T) {
	// Моделируем серверы, из которых только один доступен.
	servers := []string{"X", "Y", "Z"}
	r := rand.New(rand.NewSource(42))
	b := &p2cBalancer{
		servers: servers,
		counts: map[string]*int64{
			"X": new(int64), "Y": new(int64), "Z": new(int64),
		},
		health:     map[string]bool{"X": false, "Y": true, "Z": false},
		rand:       r,
		stopHealth: make(chan struct{}),
	}
	t.Run("AlwaysY", func(t *testing.T) {
		// Проверяем, что всегда выбирается доступный сервер "Y".
		for i := 0; i < 100; i++ {
			assert.Equal(t, "Y", b.Next())
		}
	})
}

// BenchmarkP2CBalancer измеряет производительность Next и сочетания Next/Increase/Decrease.
func BenchmarkP2CBalancer(b *testing.B) {
	// Настроим балансировщик для тестирования производительности.
	servers := []string{"one", "two", "three"}
	bl := NewP2CBalancer(servers, 500*time.Millisecond)
	defer bl.Stop()

	b.Run("NextOnly", func(b *testing.B) {
		// Измеряем производительность только для вызова Next.
		for i := 0; i < b.N; i++ {
			_ = bl.Next()
		}
	})

	b.Run("NextIncreaseDecrease", func(b *testing.B) {
		// Измеряем производительность для последовательных вызовов Next, Increase и Decrease.
		for i := 0; i < b.N; i++ {
			srv := bl.Next()
			bl.Increase(srv)
			bl.Decrease(srv)
		}
	})
}
