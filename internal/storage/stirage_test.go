package storage

import (
	"testing"
)

func BenchmarkMemStorageUpdateGauge(b *testing.B) {
	storage := NewMemStorage()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.UpdateGauge("test", float64(i))
	}
}

func BenchmarkMemStorageGetAll(b *testing.B) {
	storage := NewMemStorage()
	for i := 0; i < 100; i++ {
		storage.UpdateGauge("test", float64(i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.GetAll()
	}
}
