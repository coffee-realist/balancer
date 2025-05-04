package ratelimiter

import (
	"sync"
	"time"
)

// RateLimiter интерфейс для ограничения доступа.
type RateLimiter interface {
	Allow(key string) bool // Разрешение доступа по ключу.
	Stop()                 // Остановка работы лимитера.
}

// TokenBucketLimiter реализует алгоритм лимитирования токенами с использованием токен-бакета.
type TokenBucketLimiter struct {
	capacity       float64       // Максимальное количество токенов.
	refillTokens   float64       // Количество токенов, добавляемых за тик.
	refillInterval time.Duration // Интервал между тиками.
	buckets        sync.Map      // Хранение токенов для каждого ключа.
	stopCh         chan struct{} // Канал для остановки горутины пополнения.
}

// bucket представляет собой структуру с токенами и мьютексом для синхронизации.
type bucket struct {
	tokens float64    // Количество токенов в бакете.
	mu     sync.Mutex // Мьютекс для синхронизации доступа к токенам.
}

// NewTokenBucketLimiter создаёт новый лимитер с заданной вместимостью и интервалом пополнения.
func NewTokenBucketLimiter(capacity, refillRate float64, refillInterval time.Duration) *TokenBucketLimiter {
	tb := &TokenBucketLimiter{
		capacity: capacity,
		// Расчёт токенов, которые будут добавляться за один тик.
		refillTokens:   refillRate * refillInterval.Seconds(),
		refillInterval: refillInterval,
		stopCh:         make(chan struct{}),
	}
	// Запуск горутины для пополнения токенов.
	go tb.startRefill()
	return tb
}

// Allow проверяет и уменьшает количество токенов для указанного ключа.
func (tb *TokenBucketLimiter) Allow(key string) bool {
	// Загружаем или создаём бакет для ключа.
	actual, _ := tb.buckets.LoadOrStore(key, &bucket{tokens: tb.capacity})
	b := actual.(*bucket)

	b.mu.Lock()
	defer b.mu.Unlock()
	// Если токенов достаточно, уменьшаем их количество и разрешаем доступ.
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// startRefill регулярно пополняет токены для всех клиентов.
func (tb *TokenBucketLimiter) startRefill() {
	ticker := time.NewTicker(tb.refillInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Пополнение токенов для всех бакетов.
			tb.buckets.Range(func(_, v interface{}) bool {
				b := v.(*bucket)
				b.mu.Lock()
				b.tokens += tb.refillTokens
				// Ограничиваем количество токенов вместимостью бакета.
				if b.tokens > tb.capacity {
					b.tokens = tb.capacity
				}
				b.mu.Unlock()
				return true
			})
		case <-tb.stopCh:
			return
		}
	}
}

// Stop завершает работу лимитера, останавливая горутину пополнения.
func (tb *TokenBucketLimiter) Stop() {
	close(tb.stopCh)
}
