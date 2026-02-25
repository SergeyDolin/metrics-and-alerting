package storage

import (
	"sync"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

type MemStorage struct {
	gauge   map[string]float64
	counter map[string]int64
	mu      sync.RWMutex
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
}

func (s *MemStorage) UpdateGauge(name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gauge[name] = value
	return nil
}

func (s *MemStorage) UpdateCounter(name string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter[name] += delta
	return nil
}

func (s *MemStorage) SetCounter(name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter[name] = value
	return nil
}

func (s *MemStorage) GetGauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.gauge[name]
	return v, ok
}

func (s *MemStorage) GetCounter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.counter[name]
	return v, ok
}

func (s *MemStorage) GetAll() ([]metrics.Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []metrics.Metrics
	for name, v := range s.gauge {
		val := v
		out = append(out, metrics.Metrics{ID: name, MType: "gauge", Value: &val})
	}
	for name, d := range s.counter {
		delta := d
		out = append(out, metrics.Metrics{ID: name, MType: "counter", Delta: &delta})
	}
	return out, nil
}
