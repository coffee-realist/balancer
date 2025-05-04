package integration

import (
	"context"
	"github.com/coffee-realist/balancer/internal/balancer/adapter"
	"github.com/coffee-realist/balancer/internal/balancer/least_conn"
	"github.com/coffee-realist/balancer/internal/balancer/p2c"
	"github.com/coffee-realist/balancer/internal/balancer/round_robin"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coffee-realist/balancer/internal/logger"
	"github.com/coffee-realist/balancer/internal/proxy"
	"github.com/stretchr/testify/assert"
)

// startServer запускает HTTP-сервер на свободном локальном порту и возвращает его адрес, функцию остановки и ошибку.
func startServer(handler http.Handler) (addr string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // слушаем на случайном доступном порту
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: handler}
	go func() {
		// асинхронно запускаем сервер
		err := srv.Serve(ln)
		if err != nil {
			return
		}
	}()
	return ln.Addr().String(), func() {
		// корректное завершение работы сервера с таймаутом
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := srv.Shutdown(ctx)
		if err != nil {
			return
		}
	}, nil
}

// TestIntegration_AdaptiveModes проверяет поведение адаптивного балансировщика при различных режимах нагрузки.
func TestIntegration_AdaptiveModes(t *testing.T) {
	const total = 15 // общее количество запросов
	const (
		lowThresh  = 5  // порог переключения с RoundRobin на P2C
		highThresh = 10 // порог переключения с P2C на LeastConn
	)

	// создаём два тестовых бэкенда, возвращающих "B1" и "B2" соответственно
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, "B1")
		require.NoError(t, err)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, "B2")
		require.NoError(t, err)
	}))
	defer backend2.Close()

	// создаём адаптивный балансировщик с тремя режимами: RR, P2C, LeastConn
	rr := round_robin.NewRoundRobinBalancer([]string{backend1.URL, backend2.URL})
	lc := least_conn.NewLeastConnBalancer([]string{backend1.URL, backend2.URL})
	p2cb := p2c.NewP2CBalancer([]string{backend1.URL, backend2.URL}, 10*time.Second)
	ab := adapter.NewAdaptiveBalancer(rr, lc, p2cb, lowThresh, highThresh)
	defer ab.Stop() // корректное завершение фоновых процессов

	// создаём прокси-сервер с балансировщиком
	handler := proxy.NewProxy(ab, logger.New()).Handler()
	addr, stop, err := startServer(handler)
	assert.NoError(t, err)
	defer stop()

	client := &http.Client{Timeout: time.Second}

	// отправляем 15 последовательных запросов
	results := make([]string, total)
	for i := 0; i < total; i++ {
		resp, err := client.Get("http://" + addr + "/foo")
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		err = resp.Body.Close()
		require.NoError(t, err)
		results[i] = string(body)
	}

	// --- 1) Проверка режима RoundRobin: первые lowThresh запросов
	for i := 0; i < lowThresh; i++ {
		// ответы должны чередоваться (B1, B2, B1, ...)
		if i > 0 {
			assert.NotEqual(t, results[i], results[i-1], "RR: элементы должны чередоваться")
		}
	}

	// --- 2) Проверка режима P2C: запросы [lowThresh..highThresh)
	// проверяем, что оба бэкенда используются хотя бы раз
	seen1, seen2 := false, false
	for i := lowThresh; i < highThresh; i++ {
		if results[i] == "B1" {
			seen1 = true
		}
		if results[i] == "B2" {
			seen2 = true
		}
	}
	assert.True(t, seen1, "P2C: B1 должен получить хотя бы один запрос")
	assert.True(t, seen2, "P2C: B2 должен получить хотя бы один запрос")

	// --- 3) Проверка режима LeastConn: последние запросы
	// так как соединения краткоживущие, LC всегда будет выбирать первый
	for i := highThresh; i < total; i++ {
		assert.Equal(t, "B1", results[i], "LC: всегда выбираем первый сервер")
	}
}

// Benchmark_LimiterThroughput — бенчмарк, измеряющий производительность клиента при высокой нагрузке.
func Benchmark_LimiterThroughput(b *testing.B) {
	// предполагается, что прокси-сервер уже работает по адресу localhost:8080
	url := "http://localhost:8080/foo?id=bench-client"
	client := &http.Client{Timeout: 2 * time.Second}

	// сбрасываем таймер и запускаем параллельные запросы
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(url)
			if err == nil {
				err := resp.Body.Close()
				b.Errorf("failed to close response body: %v", err)
			}
		}
	})
}
