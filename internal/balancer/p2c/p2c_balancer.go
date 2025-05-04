package p2c

import (
	"github.com/coffee-realist/balancer/internal/balancer"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// IP2CBalancer реализует балансировщик с алгоритмом Power of Two Choices,
// совмещая выбор сервера, управление соединениями и контроль здоровья бэкендов.
type IP2CBalancer interface {
	balancer.Balancer
	balancer.ConnAware
	balancer.Stoppable
}

// p2cBalancer использует алгоритм "Power of Two Choices" для выбора наименее нагруженного сервера
// из двух случайно выбранных кандидатов, учитывая текущее состояние здоровья бэкендов.
type p2cBalancer struct {
	servers    []string          // Список всех доступных бэкендов
	counts     map[string]*int64 // Атомарные счетчики активных соединений для каждого сервера
	rand       *rand.Rand        // Генератор случайных чисел для выбора кандидатов
	health     map[string]bool   // Текущее состояние здоровья серверов
	muHealth   sync.RWMutex      // RWMutex для синхронизации доступа к health map
	hcInterval time.Duration     // Интервал проверки здоровья бэкендов
	hcClient   *http.Client      // HTTP клиент для health checks
	stopHealth chan struct{}     // Канал для остановки health checks
}

// NewP2CBalancer создает новый экземпляр балансировщика с фоновыми проверками здоровья.
// hcInterval определяет частоту проверок работоспособности бэкендов.
func NewP2CBalancer(servers []string, hcInterval time.Duration) IP2CBalancer {
	// Инициализация счетчиков соединений
	c := make(map[string]*int64, len(servers))
	for _, s := range servers {
		var zero int64 = 0
		c[s] = &zero
	}

	// Инициализация генератора случайных чисел с уникальным сидом
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	h := make(map[string]bool, len(servers))
	for _, server := range servers {
		h[server] = true
	}

	b := &p2cBalancer{
		servers:    servers,
		counts:     c,
		rand:       r,
		health:     h,
		hcInterval: hcInterval,
		hcClient:   &http.Client{Timeout: 1 * time.Second},
		stopHealth: make(chan struct{}),
	}

	// Запуск фоновых проверок здоровья
	go b.startHealthChecks()

	return b
}

// Next реализует алгоритм выбора сервера:
// 1. Выбирает два случайных здоровых бэкенда
// 2. Возвращает сервер с наименьшим количеством активных соединений
func (b *p2cBalancer) Next() string {
	healthy := b.getHealthyServers()
	n := len(healthy)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return healthy[0]
	}

	// Выбор двух случайных кандидатов
	i1, i2 := b.rand.Intn(n), b.rand.Intn(n)
	if i1 == i2 {
		i2 = b.rand.Intn(n)
	}
	s1, s2 := healthy[i1], healthy[i2]

	// Сравнение нагрузки с использованием атомарных операций
	if atomic.LoadInt64(b.counts[s1]) <= atomic.LoadInt64(b.counts[s2]) {
		return s1
	}
	return s2
}

// getHealthyServers возвращает список доступных серверов с учетом текущего состояния здоровья
func (b *p2cBalancer) getHealthyServers() []string {
	b.muHealth.RLock()
	defer b.muHealth.RUnlock()
	healthy := make([]string, 0, len(b.servers))
	for _, s := range b.servers {
		if b.health[s] {
			healthy = append(healthy, s)
		}
	}

	return healthy
}

// startHealthChecks запускает периодические проверки здоровья бэкендов
func (b *p2cBalancer) startHealthChecks() {
	ticker := time.NewTicker(b.hcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Параллельная проверка всех серверов
			for _, srv := range b.servers {
				go b.checkOne(srv)
			}
		case <-b.stopHealth:
			return
		}
	}
}

// checkOne выполняет HTTP HEAD запрос для проверки здоровья сервера
func (b *p2cBalancer) checkOne(server string) {
	req, _ := http.NewRequest("HEAD", server+"/health", nil)
	resp, err := b.hcClient.Do(req)
	up := err == nil && resp.StatusCode < 500

	// Обновление состояния с блокировкой записи
	b.muHealth.Lock()
	b.health[server] = up
	b.muHealth.Unlock()

	if resp != nil {
		err := resp.Body.Close()
		if err != nil {
			return
		}
	}
}

// Stop корректно завершает работу health checks
func (b *p2cBalancer) Stop() {
	close(b.stopHealth)
}

// Increase атомарно увеличивает счетчик активных соединений для сервера
func (b *p2cBalancer) Increase(server string) {
	if server == "" {
		return
	}
	if ptr, ok := b.counts[server]; ok {
		atomic.AddInt64(ptr, 1)
	}
}

// Decrease атомарно уменьшает счетчик активных соединений для сервера
func (b *p2cBalancer) Decrease(server string) {
	if server == "" {
		return
	}
	if ptr, ok := b.counts[server]; ok {
		atomic.AddInt64(ptr, -1)
	}
}
