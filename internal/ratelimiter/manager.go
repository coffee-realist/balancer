package ratelimiter

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // драйвер SQLite
)

const (
	// Интервал, с которым состояние токенов будет сохраняться в БД
	defaultPersistInterval = 5 * time.Second
)

// ClientConfig описывает конфигурацию токен-бакета клиента
type ClientConfig struct {
	Capacity       float64       `json:"capacity"`        // Максимальное количество токенов
	RefillRate     float64       `json:"refill_rate"`     // Количество токенов, добавляемых за один интервал
	RefillInterval time.Duration `json:"refill_interval"` // Интервал пополнения
	Tokens         float64       `json:"tokens"`          // Текущее количество токенов
}

// tokenBucketClient — структура одного клиента с токен-бакетом
type tokenBucketClient struct {
	config ClientConfig
	ticker *time.Ticker  // для периодического пополнения
	mu     sync.Mutex    // мьютекс для потокобезопасного доступа
	tokens float64       // текущее количество токенов
	stopCh chan struct{} // канал остановки refill-горутины
}

// DBManager управляет всеми клиентами и их сохранением в БД
type DBManager struct {
	db      *sql.DB                       // подключение к SQLite
	mu      sync.RWMutex                  // мьютекс для управления картой клиентов
	clients map[string]*tokenBucketClient // карта ID клиента к его структуре
	stopAll chan struct{}                 // канал остановки всех горутин
}

// NewDBManager открывает SQLite файл и загружает клиентов из БД
func NewDBManager(dbPath string) (*DBManager, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	// создаёт таблицу клиентов, если её нет
	schema := `
    CREATE TABLE IF NOT EXISTS clients (
        id             TEXT PRIMARY KEY,
        config         TEXT NOT NULL,  -- JSON сериализация ClientConfig
        tokens         REAL NOT NULL,
        last_updated   DATETIME NOT NULL
    );`

	if _, err := db.Exec(schema); err != nil {
		err := db.Close()
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	m := &DBManager{
		db:      db,
		clients: make(map[string]*tokenBucketClient),
		stopAll: make(chan struct{}),
	}
	// Загружаем состояние клиентов из БД
	if err := m.loadFromDB(); err != nil {
		err := db.Close()
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	// Запускаем фоновую синхронизацию состояния в БД
	go m.startPersistLoop()
	return m, nil
}

// loadFromDB загружает все сохранённые токен-бакеты из БД
func (m *DBManager) loadFromDB() error {
	rows, err := m.db.Query(`SELECT id, config, tokens FROM clients`)
	if err != nil {
		return err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			return
		}
	}(rows)

	for rows.Next() {
		var id, cfgJSON string
		var tokens float64

		// читаем ID, JSON конфигурацию и токены
		if err := rows.Scan(&id, &cfgJSON, &tokens); err != nil {
			return err
		}

		var cfg ClientConfig
		if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
			return err
		}

		// создаём структуру клиента и запускаем refill
		tb := &tokenBucketClient{
			config: cfg,
			tokens: tokens,
			ticker: time.NewTicker(cfg.RefillInterval),
			stopCh: make(chan struct{}),
		}
		m.clients[id] = tb
		go m.runRefill(tb)
	}
	return rows.Err()
}

// AddClient добавляет или обновляет клиента в памяти и в БД
func (m *DBManager) AddClient(id string, cfg ClientConfig) error {
	m.mu.Lock()

	// если клиент уже есть, останавливаем его refill-горутину
	if old, ok := m.clients[id]; ok {
		close(old.stopCh)
		old.ticker.Stop()
	}

	// создаём нового клиента
	tb := &tokenBucketClient{
		config: cfg,
		tokens: cfg.Capacity,
		ticker: time.NewTicker(cfg.RefillInterval),
		stopCh: make(chan struct{}),
	}
	m.clients[id] = tb
	m.mu.Unlock()

	// запускаем refill-горутину
	go m.runRefill(tb)

	// сохраняем клиента в БД
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	tb.mu.Lock()
	tokens := tb.tokens
	tb.mu.Unlock()

	_, err = m.db.Exec(`
		INSERT INTO clients(id, config, tokens, last_updated)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			config=excluded.config,
			tokens=excluded.tokens,
			last_updated=CURRENT_TIMESTAMP
	`, id, string(cfgJSON), tokens)

	return err
}

// Allow проверяет наличие токенов и уменьшает их количество
func (m *DBManager) Allow(id string) bool {
	m.mu.RLock()
	tb, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// GetClient возвращает конфигурацию клиента и текущее количество токенов
func (m *DBManager) GetClient(id string) (ClientConfig, error) {
	m.mu.RLock()
	tb, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok {
		return ClientConfig{}, errors.New("not found")
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	cfg := tb.config
	cfg.Tokens = tb.tokens
	return cfg, nil
}

// RemoveClient удаляет клиента из памяти и из базы данных
func (m *DBManager) RemoveClient(id string) error {
	m.mu.Lock()
	tb, ok := m.clients[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("not found")
	}
	close(tb.stopCh)
	tb.ticker.Stop()

	// удаляем из БД
	_, err := m.db.Exec(`DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}

	// удаляем из памяти
	m.mu.Lock()
	delete(m.clients, id)
	m.mu.Unlock()
	return nil
}

// runRefill запускает горутину, которая пополняет токены по таймеру
func (m *DBManager) runRefill(tb *tokenBucketClient) {
	cfg := tb.config
	for {
		select {
		case <-tb.ticker.C:
			tb.mu.Lock()
			tb.tokens += cfg.RefillRate * cfg.RefillInterval.Seconds()
			if tb.tokens > cfg.Capacity {
				tb.tokens = cfg.Capacity
			}
			tb.mu.Unlock()
		case <-tb.stopCh:
			return
		}
	}
}

// startPersistLoop периодически сохраняет текущее состояние всех клиентов в БД
func (m *DBManager) startPersistLoop() {
	ticker := time.NewTicker(defaultPersistInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.RLock()
			for id, tb := range m.clients {
				tb.mu.Lock()
				tokens := tb.tokens
				cfgJSON, _ := json.Marshal(tb.config)
				tb.mu.Unlock()
				_, err := m.db.Exec(`
                    UPDATE clients
                    SET tokens = ?, last_updated = CURRENT_TIMESTAMP
                    WHERE id = ?
                `, tokens, id)
				if err != nil {
					return
				}
				_ = cfgJSON // сохранённый конфиг пока не используется
			}
			m.mu.RUnlock()
		case <-m.stopAll:
			return
		}
	}
}

// Stop завершает все фоновые refill-горутины и закрывает соединение с БД
func (m *DBManager) Stop() {
	m.mu.Lock()
	for _, tb := range m.clients {
		close(tb.stopCh)
		tb.ticker.Stop()
	}
	close(m.stopAll)
	err := m.db.Close()
	if err != nil {
		return
	}
	m.mu.Unlock()
}
