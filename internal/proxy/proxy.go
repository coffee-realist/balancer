package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/coffee-realist/balancer/internal/balancer"
	"github.com/coffee-realist/balancer/internal/logger"
)

// Proxy инкапсулирует проксирующую логику и использует балансировщик для выбора сервера.
type Proxy struct {
	balancer  balancer.Balancer // Интерфейс балансировщика
	logger    logger.Logger     // Логгер для вывода служебной информации
	transport http.RoundTripper // HTTP-транспорт для выполнения запросов (можно переопределить)
}

// NewProxy создает новый экземпляр Proxy с указанным балансировщиком и логгером.
func NewProxy(b balancer.Balancer, log logger.Logger) *Proxy {
	return &Proxy{
		balancer:  b,
		logger:    log,
		transport: http.DefaultTransport,
	}
}

// Handler возвращает http.Handler, который проксирует запросы на серверы, выбранные балансировщиком.
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Выбор сервера через балансировщик
		server := p.balancer.Next()
		if server == "" {
			http.Error(w, "no servers available", http.StatusServiceUnavailable)
			return
		}

		// Увеличиваем счетчик соединений, если балансировщик поддерживает ConnAware
		if ca, ok := p.balancer.(balancer.ConnAware); ok {
			ca.Increase(server)
			defer ca.Decrease(server)
		}

		p.logger.Infof("proxying %s %s -> %s", r.Method, r.URL.String(), server)

		// Парсинг целевого URL
		targetURL, err := url.Parse(server)
		if err != nil {
			p.logger.Errorf("invalid server URL %q: %v", server, err)
			http.Error(w, "bad server URL", http.StatusInternalServerError)
			return
		}

		// Создание и настройка reverse proxy
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = targetURL.Scheme
				req.URL.Host = targetURL.Host
			},
			Transport: p.transport,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				p.logger.Errorf("server %s error: %v", server, err)
				http.Error(w, "bad gateway", http.StatusBadGateway)
			},
		}

		// Прокси обработка запроса
		proxy.ServeHTTP(w, r)
	})
}
