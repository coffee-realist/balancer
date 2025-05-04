package balancer

// Balancer — минимальный интерфейс, возвращает следующий сервер.
type Balancer interface {
	Next() string
}

// ConnAware — для стратегий, которым нужно отслеживать подключения.
type ConnAware interface {
	// Increase увеличивает счётчик активных соединений на сервере.
	Increase(server string)
	// Decrease уменьшает счётчик активных соединений.
	Decrease(server string)
}

// Stoppable — для стратегий, у которых есть фоновые горутины (health-checks, refill).
type Stoppable interface {
	Stop()
}
