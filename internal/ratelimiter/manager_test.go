package ratelimiter

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSQLitePersistence проверяет сохранение и восстановление данных о клиентах из базы данных.
func TestSQLitePersistence(t *testing.T) {
	// Создаём временный файл базы данных для теста
	path := "test_clients.db"
	defer func() {
		// Удаляем файл базы данных после теста
		err := os.Remove(path)
		if err != nil {
			t.Errorf("error removing temporary test clients file: %v", err)
		}
	}()

	// Инициализируем менеджер БД и добавляем клиента
	mgr, err := NewDBManager(path)
	assert.NoError(t, err)
	cfg := ClientConfig{Capacity: 4, RefillRate: 2, RefillInterval: 100 * time.Millisecond}
	assert.NoError(t, mgr.AddClient("u1", cfg))

	// Даем время на синхронизацию
	time.Sleep(defaultPersistInterval + 50*time.Millisecond)

	// Проверяем восстановление состояния после перезапуска
	mgr2, err := NewDBManager(path)
	assert.NoError(t, err)
	defer mgr2.Stop()

	got, err := mgr2.GetClient("u1")
	assert.NoError(t, err)
	assert.Equal(t, cfg.Capacity, got.Capacity)
}
