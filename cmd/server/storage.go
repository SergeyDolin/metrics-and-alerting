package main

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// MetricStorage — структура для хранения метрик двух типов: gauge (произвольное значение) и counter (счётчик, только инкремент)
type MetricStorage struct {
	gauge   map[string]float64 // Хранит метрики типа gauge (например использование памяти)
	counter map[string]int64   // Хранит метрики типа counter (например количество запросов или ошибок)
	mu      sync.Mutex
	db      *sql.DB
}

// createMetricStorage — создаёт и инициализирует новый экземпляр хранилища метрик.
// Возвращает указатель на MetricStorage с инициализированными пустыми мапами для gauge и counter.
func createMetricStorage(dsn string) (*MetricStorage, error) {
	ms := &MetricStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}

	if dsn != "" {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open DB: %w", err)
		}
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping DB: %w", err)
		}

		ms.db = db

		if err := ms.createTables(dsn); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to init DB schema: %w", err)
		}
	}
	return ms, nil
}
