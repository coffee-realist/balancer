package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/coffee-realist/balancer/internal/balancer"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coffee-realist/balancer/internal/api"
	"github.com/coffee-realist/balancer/internal/balancer/adapter"
	"github.com/coffee-realist/balancer/internal/balancer/least_conn"
	"github.com/coffee-realist/balancer/internal/balancer/p2c"
	"github.com/coffee-realist/balancer/internal/balancer/round_robin"
	"github.com/coffee-realist/balancer/internal/config"
	"github.com/coffee-realist/balancer/internal/logger"
	"github.com/coffee-realist/balancer/internal/proxy"
	"github.com/coffee-realist/balancer/internal/ratelimiter"
)

// Start инициализирует и запускает HTTP сервер с необходимыми компонентами.
func Start(cfg *config.Config) error {
	log := logger.New()

	// Настройка RateLimiter с параметрами из конфигурации.
	rlCfg := cfg.RateLimiter
	if rlCfg.Capacity <= 0 {
		rlCfg.Capacity = 100
	}
	if rlCfg.RefillRate <= 0 {
		rlCfg.RefillRate = 50
	}
	if rlCfg.RefillInterval <= 0 {
		rlCfg.RefillInterval = time.Second
	}
	// Инициализация глобального лимитера токенов.
	globalRL := ratelimiter.NewTokenBucketLimiter(
		rlCfg.Capacity, rlCfg.RefillRate, rlCfg.RefillInterval,
	)
	defer globalRL.Stop()

	// Инициализация DBManager для работы с базой данных.
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "clients.db"
		log.Infof("database path not set, using default db path '%s'", dbPath)
	}
	dbMgr, err := ratelimiter.NewDBManager(dbPath)
	if err != nil {
		log.Errorf("FATAL: db initialization failed: %v", err)
		return fmt.Errorf("database initialization failed: %w", err)
	}
	defer dbMgr.Stop()

	// Выбор алгоритма балансировки на основе конфигурации.
	hcInterval := cfg.HealthCheckInterval
	if hcInterval <= 0 {
		hcInterval = 2 * time.Second
	}

	var bal balancer.Balancer
	switch cfg.Algorithm {
	case "rr":
		bal = round_robin.NewRoundRobinBalancer(cfg.Servers)
	case "lc":
		bal = least_conn.NewLeastConnBalancer(cfg.Servers)
	case "p2c":
		bal = p2c.NewP2CBalancer(cfg.Servers, hcInterval)
	case "adaptive":
		// Инициализация адаптивного балансировщика, комбинирующего несколько алгоритмов.
		rr := round_robin.NewRoundRobinBalancer(cfg.Servers)
		lc := least_conn.NewLeastConnBalancer(cfg.Servers)
		p2cb := p2c.NewP2CBalancer(cfg.Servers, hcInterval)
		low, high := cfg.Adaptive.LowThreshold, cfg.Adaptive.HighThreshold
		if low < 0 {
			low = 10
		}
		if high <= low {
			high = low * 10
		}
		ab := adapter.NewAdaptiveBalancer(rr, lc, p2cb, low, high)
		defer ab.Stop()
		bal = ab
	default:
		bal = round_robin.NewRoundRobinBalancer(cfg.Servers)
	}

	// Инициализация Proxy с выбранным балансировщиком.
	prox := proxy.NewProxy(bal, log)

	// Инициализация HTTP роутера и middleware.
	mux := http.NewServeMux()
	// Регистрация API с передачей DBManager и обработчика прокси.
	api.Register(mux, dbMgr, prox.Handler(), log)
	handler := loggingMiddleware(mux, log)
	handler = rateLimitMiddleware(handler, globalRL, dbMgr, log)

	// Запуск HTTP-сервера с поддержкой graceful shutdown.
	srv := &http.Server{
		Addr:    cfg.ListenPort,
		Handler: handler,
	}

	// Запуск сервера в отдельной горутине.
	go func() {
		log.Infof("starting HTTP server on %s", cfg.ListenPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("server error: %v", err)
		}
	}()

	// Ожидание сигнала для graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Infof("shutdown signal received, shutting down...")

	// Ожидание завершения работы сервера.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// rateLimitMiddleware реализует логику ограничения скорости для всех запросов.
func rateLimitMiddleware(
	next http.Handler,
	globalRL ratelimiter.RateLimiter,
	clientRL ratelimiter.RateLimiter,
	log logger.Logger,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Применение глобального ограничения по IP-адресу.
		if !globalRL.Allow(r.RemoteAddr) {
			log.Errorf("rate limit exceeded: %s", r.RemoteAddr)
			http.Error(w, `{"code":429,"message":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		// Применение per-client ограничения по API-Key.
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if !clientRL.Allow(apiKey) {
				log.Errorf("rate limit exceeded: %s", r.RemoteAddr)
				http.Error(w, `{"code":429,"message":"client rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware логирует входящие HTTP-запросы и время их обработки.
func loggingMiddleware(next http.Handler, log logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Infof("incoming request: %s %s", r.Method, r.URL.String())
		next.ServeHTTP(w, r)
		log.Infof("completed %s %s in %v", r.Method, r.URL.String(), time.Since(start))
	})
}
