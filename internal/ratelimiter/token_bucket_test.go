package ratelimiter

import (
	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// setupLimiter инициализирует лимитер с заданными параметрами и возвращает функцию для остановки.
func setupLimiter(cap, rate float64, interval time.Duration) (rl *TokenBucketLimiter, teardown func()) {
	rl = NewTokenBucketLimiter(cap, rate, interval)

	return rl, func() { rl.Stop() }
}

// TestTokenBucketLimiter_ConsumeInitialTokens тестирует потребление начальных токенов лимитером.
func TestTokenBucketLimiter_ConsumeInitialTokens(t *testing.T) {
	rl, teardown := setupLimiter(5, 1, 100*time.Second)
	defer teardown()

	client := gofakeit.UUID()
	t.Run("use up all initial tokens", func(t *testing.T) {
		// Проверка доступности токенов до достижения емкости.
		for i := 0; i < 5; i++ {
			assert.True(t, rl.Allow(client), "token %d should be available", i+1)
		}
	})
	t.Run("sixth request is denied", func(t *testing.T) {
		// Проверка, что шестой запрос отклоняется, когда бакет пуст.
		assert.False(t, rl.Allow(client), "bucket should be empty after capacity requests")
	})
}

// TestTokenBucketLimiter_RefillLogic тестирует логику пополнения токенов.
func TestTokenBucketLimiter_RefillLogic(t *testing.T) {
	rl, teardown := setupLimiter(2, 2, 200*time.Millisecond)
	defer teardown()

	client := "client-refill"
	assert.True(t, rl.Allow(client))               // Первый запрос должен быть разрешен.
	assert.True(t, rl.Allow(client))               // Второй запрос должен быть разрешен.
	assert.False(t, rl.Allow(client), "now empty") // Бакет пуст.

	t.Run("no refill before first interval", func(t *testing.T) {
		// Проверка, что пополнение не происходит до первого интервала.
		time.Sleep(150 * time.Millisecond)
		assert.False(t, rl.Allow(client))
	})

	t.Run("refill adds tokens correctly", func(t *testing.T) {
		// Проверка, что после интервала токены добавляются правильно.
		time.Sleep(450 * time.Millisecond)
		assert.True(t, rl.Allow(client), "after sufficient time bucket should have >= 1 token")
		assert.False(t, rl.Allow(client))
	})
}

// TestTokenBucketLimiter_Refill тестирует, что токены не разделяются между клиентами.
func TestTokenBucketLimiter_Refill(t *testing.T) {
	rl, teardown := setupLimiter(1, 1, 100*time.Second)
	defer teardown()

	clientA := "A"
	clientB := "B"

	t.Run("different clients don't share tokens", func(t *testing.T) {
		// Проверка, что токены для разных клиентов не делятся.
		assert.True(t, rl.Allow(clientA))
		assert.False(t, rl.Allow(clientA), "A empty")

		assert.True(t, rl.Allow(clientB))
		assert.False(t, rl.Allow(clientB), "B empty")
	})
}

// BenchmarkTokenBucketLimiter_Allow производит бенчмаркинг метода Allow.
func BenchmarkTokenBucketLimiter_Allow(b *testing.B) {
	rl := NewTokenBucketLimiter(1000, 500, time.Millisecond)
	defer rl.Stop()

	client := "bench-client"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(client) // Оценка производительности метода Allow.
	}
}
