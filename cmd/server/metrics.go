package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/pgerrors"
)

// updateGauge — обновляет или устанавливает значение метрики типа gauge по имени.
// Перезаписывает текущее значение, если оно существует.
func (ms *MetricStorage) updateGauge(name string, value float64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.gauge[name] = value
}

// updateCounter — обновляет значение метрики типа counter по имени.
// Если метрика ещё не существует — инициализирует её нулём, затем прибавляет переданное значение.
// Counter предназначен для накопления, а не перезаписи.
func (ms *MetricStorage) updateCounter(name string, value int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.counter[name]; !ok {
		ms.counter[name] = 0
	}
	ms.counter[name] += value
}

func (ms *MetricStorage) SaveToFile(filePath string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var allMetrics []metrics.Metrics

	for name, value := range ms.gauge {
		v := value
		allMetrics = append(allMetrics, metrics.Metrics{
			ID:    name,
			MType: "gauge",
			Value: &v,
		})
	}
	for name, value := range ms.counter {
		d := value
		allMetrics = append(allMetrics, metrics.Metrics{
			ID:    name,
			MType: "counter",
			Delta: &d,
		})
	}

	data, err := json.MarshalIndent(allMetrics, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_SYNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func (ms *MetricStorage) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var metrics []metrics.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	for _, m := range metrics {
		switch m.MType {
		case "gauge":
			if m.Value != nil {
				ms.gauge[m.ID] = *m.Value
			}
		case "counter":
			if m.Delta != nil {
				ms.counter[m.ID] = *m.Delta
			}
		}
	}
	return nil
}

func (ms *MetricStorage) createTables(dbName string) error {
	_, err := ms.db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS gauge (
			name TEXT PRIMARY KEY,
			value DOUBLE PRECISION NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("create gauge table: %w", err)
	}

	_, err = ms.db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS counter (
			name TEXT PRIMARY KEY,
			value BIGINT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("create counter table: %w", err)
	}

	return nil
}

func (ms *MetricStorage) saveToDB() {
	classifier := pgerrors.NewPostgresErrorClassifier()

	backoffDelays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}
	maxRetries := len(backoffDelays)

	ms.mu.Lock()
	defer ms.mu.Unlock()

	gaugeMetrics := make(map[string]float64)
	for k, v := range ms.gauge {
		gaugeMetrics[k] = v
	}
	counterMetrics := make(map[string]int64)
	for k, v := range ms.counter {
		counterMetrics[k] = v
	}

	saveBatch := func(query string, metrics map[string]interface{}) error {
		for name, value := range metrics {
			var err error
			for attempt := 0; attempt <= maxRetries; attempt++ {
				_, err = ms.db.ExecContext(context.Background(), query, name, value)
				if err == nil {
					break // успех — выходим
				}

				// Проверяем, является ли ошибка повторяемой
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					if classifier.Classify(err) == pgerrors.Retriable {
						if attempt < maxRetries {
							time.Sleep(backoffDelays[attempt])
							continue
						}
					}
				}
				return err
			}
		}
		return nil
	}

	err := saveBatch(
		`INSERT INTO gauge (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`,
		func(m map[string]float64) map[string]interface{} {
			result := make(map[string]interface{})
			for k, v := range m {
				result[k] = v
			}
			return result
		}(gaugeMetrics),
	)
	if err != nil {
		fmt.Printf("Failed to save gauge metrics after retries: %v\n", err)
	}
	err = saveBatch(
		`INSERT INTO counter (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`,
		func(m map[string]int64) map[string]interface{} {
			result := make(map[string]interface{})
			for k, v := range m {
				result[k] = v
			}
			return result
		}(counterMetrics),
	)
	if err != nil {
		fmt.Printf("Failed to save counter metrics after retries: %v\n", err)
	}
}

func (ms *MetricStorage) loadFromDB() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	rows, err := ms.db.QueryContext(context.Background(), `SELECT name, value FROM gauge`)
	if err != nil {
		return fmt.Errorf("query gauge: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var value float64
		if err := rows.Scan(&name, &value); err != nil {
			return fmt.Errorf("scan gauge: %w", err)
		}
		ms.gauge[name] = value
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate gauge rows: %w", err)
	}

	rows, err = ms.db.QueryContext(context.Background(), `SELECT name, value FROM counter`)
	if err != nil {
		return fmt.Errorf("query counter: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var value int64
		if err := rows.Scan(&name, &value); err != nil {
			return fmt.Errorf("scan counter: %w", err)
		}
		ms.counter[name] = value
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate counter rows: %w", err)
	}

	return nil
}
