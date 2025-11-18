package storage

import "github.com/SergeyDolin/metrics-and-alerting/internal/metrics"

type Storage interface {
	UpdateGauge(name string, value float64) error
	UpdateCounter(name string, delta int64) error
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
	GetAll() ([]metrics.Metrics, error)
}
