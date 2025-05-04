package api

import (
	"bytes"
	"encoding/json"
	"github.com/coffee-realist/balancer/internal/ratelimiter"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// dummyLog — пустая реализация logger.Logger для использования в тестах.
type dummyLog struct{}

func (dummyLog) Infof(string, ...interface{})  {}
func (dummyLog) Errorf(string, ...interface{}) {}

// setupAPI — настройка тестового API-сервера и менеджера клиентов.
func setupAPI() (*httptest.Server, *ratelimiter.DBManager) {
	mux := http.NewServeMux()
	clientMgr, err := ratelimiter.NewDBManager("test_clients.db")
	if err != nil {
		panic(err)
	}
	Register(mux, clientMgr, http.NotFoundHandler(), dummyLog{})
	ts := httptest.NewServer(mux)
	return ts, clientMgr
}

// TestAPIClientCRUD — тестирование CRUD операций для клиентов API.
func TestAPIClientCRUD(t *testing.T) {
	ts, mgr := setupAPI()
	defer ts.Close()
	defer mgr.Stop()

	id := "client1"
	cfg := ratelimiter.ClientConfig{
		Capacity:       2,
		RefillRate:     1,
		RefillInterval: 100 * time.Millisecond,
	}
	body, _ := json.Marshal(cfg)

	// Тест для создания клиента
	t.Run("CreateClient", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/clients?id="+id, "application/json", bytes.NewReader(body))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Тест для получения клиента
	t.Run("GetClient", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/clients/" + id)
		assert.NoError(t, err)

		var got ratelimiter.ClientConfig
		err = json.NewDecoder(resp.Body).Decode(&got)
		assert.NoError(t, err)
		assert.Equal(t, cfg.Capacity, got.Capacity)
		assert.Equal(t, cfg.RefillRate, got.RefillRate)
	})

	// Тест для удаления клиента
	t.Run("DeleteClient", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/clients/"+id, nil)
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Тест для получения несуществующего клиента
	t.Run("GetMissing", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/clients/" + id)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// BenchmarkAPIAllow — бенчмаркинг метода Allow.
func BenchmarkAPIAllow(b *testing.B) {
	ts, mgr := setupAPI()
	defer ts.Close()
	defer mgr.Stop()

	id := "bench"
	cfg := ratelimiter.ClientConfig{Capacity: 1000, RefillRate: 500, RefillInterval: time.Second}
	_, err := http.Post(ts.URL+"/clients?id="+id, "application/json", bytes.NewReader(func() []byte {
		j, _ := json.Marshal(cfg)
		return j
	}()))
	if err != nil {
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.Allow(id)
	}
}
