package main

import (
	"encoding/json"
	"os"
	"syscall"
)

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

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
	var metrics []Metrics

	for name, value := range ms.gauge {
		v := value
		metrics = append(metrics, Metrics{
			ID:    name,
			MType: "gauge",
			Value: &v,
		})
	}
	for name, value := range ms.counter {
		d := value
		metrics = append(metrics, Metrics{
			ID:    name,
			MType: "counter",
			Delta: &d,
		})
	}

	data, err := json.MarshalIndent(metrics, "", "  ")
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

	var metrics []Metrics
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
