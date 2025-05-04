package server

import (
	"github.com/coffee-realist/balancer/internal/config"
	"os"
	"testing"
	"time"
)

// TestServerStartStop проверяет корректное завершение работы сервера.
func TestServerStartStop(t *testing.T) {
	// Тест на корректное завершение работы сервера по сигналу.
	t.Run("shutdown test", func(t *testing.T) {
		cfg := &config.Config{
			ListenPort: ":0", // Случайный порт для теста
			Servers:    []string{},
		}
		done := make(chan error, 1)

		go func() {
			done <- Start(cfg)
		}()

		time.Sleep(100 * time.Millisecond)

		// Отправка сигнала прерывания.
		p, _ := os.FindProcess(os.Getpid())
		err := p.Signal(os.Interrupt)
		if err != nil {
			t.Error("failed to send interrupt signal")
			return
		}

		// Ожидание завершения сервера.
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("server shutdown with error: %v", err)
			}

		case <-time.After(2 * time.Second):
			t.Fatal("server did not shutdown in time")
		}
	})
}
