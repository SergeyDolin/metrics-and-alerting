package storage

import (
	"encoding/json"
	"os"
	"sync"
	"syscall"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
)

type FileStorage struct {
	*MemStorage
	filePath string
	mu       sync.Mutex
}

func NewFileStorage(filePath string) (*FileStorage, error) {
	s := &FileStorage{
		MemStorage: NewMemStorage(),
		filePath:   filePath,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStorage) UpdateGauge(name string, value float64) error {
	if err := s.MemStorage.UpdateGauge(name, value); err != nil {
		return err
	}
	return s.Save()
}

func (s *FileStorage) UpdateCounter(name string, delta int64) error {
	if err := s.MemStorage.UpdateCounter(name, delta); err != nil {
		return err
	}
	return s.Save()
}

func (s *FileStorage) SetCounter(name string, value int64) error {
	if err := s.MemStorage.SetCounter(name, value); err != nil {
		return err
	}
	return s.Save()
}

func (s *FileStorage) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metricsList, err := s.MemStorage.GetAll()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(metricsList, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(s.filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_SYNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func (s *FileStorage) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var metricsList []metrics.Metrics
	if err := json.Unmarshal(data, &metricsList); err != nil {
		return err
	}

	for _, m := range metricsList {
		switch m.MType {
		case "gauge":
			if m.Value != nil {
				s.gauge[m.ID] = *m.Value
			}
		case "counter":
			if m.Delta != nil {
				s.counter[m.ID] = *m.Delta
			}
		}
	}
	return nil
}
