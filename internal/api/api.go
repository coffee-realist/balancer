package api

import (
	"encoding/json"
	"github.com/coffee-realist/balancer/internal/ratelimiter"
	"net/http"
	"strings"

	"github.com/coffee-realist/balancer/internal/logger"
)

// Register регистрирует HTTP-обработчики для управления клиентами и проксирования запросов.
// - /clients используется для добавления клиента (POST),
// - /clients/<id> — для получения (GET) и удаления (DELETE),
// - все остальные запросы обрабатываются через proxyHandler.
func Register(
	mux *http.ServeMux,
	clientMgr *ratelimiter.DBManager,
	proxyHandler http.Handler,
	log logger.Logger,
) {
	// Обработчик для POST-запросов на /clients: добавление нового клиента по ID
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		if clientMgr == nil {
			http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		// Разрешаем только POST-метод
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Получаем ID клиента из query-параметра
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
			return
		}

		// Декодируем конфигурацию клиента из тела запроса
		var cfg ratelimiter.ClientConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
			return
		}

		// Пытаемся добавить клиента в менеджер
		err := clientMgr.AddClient(id, cfg)
		if err != nil {
			log.Errorf("add client error: %v", err)
			return
		}

		// Успешное добавление — 201 Created
		w.WriteHeader(http.StatusCreated)
	})

	// Обработчик для GET и DELETE-запросов на /clients/<id>
	mux.HandleFunc("/clients/", func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем ID из URL-пути, ожидается "/clients/<id>"
		parts := strings.SplitN(r.URL.Path, "/", 3)
		if len(parts) != 3 || parts[2] == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		id := parts[2]

		switch r.Method {
		case http.MethodGet:
			// Получаем конфигурацию клиента по ID
			cfg, err := clientMgr.GetClient(id)
			if err != nil {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}

			// Отправляем конфигурацию в формате JSON
			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(cfg)
			if err != nil {
				log.Errorf("encode client error: %v", err)
				return
			}

		case http.MethodDelete:
			// Удаляем клиента по ID
			if err := clientMgr.RemoveClient(id); err != nil {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent) // 204 No Content при успешном удалении

		default:
			// Любые другие методы не разрешены
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Все остальные пути обрабатываются прокси-обработчиком
	mux.Handle("/", proxyHandler)
}
