package storage

import (
	"context"
	"testing"
)

func BenchmarkMemStorageUpdateGauge(b *testing.B, ctx context.Context) {
	storage := NewMemStorage()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.UpdateGauge(ctx, "test", float64(i))
	}
}

func BenchmarkMemStorageGetAll(b *testing.B, ctx context.Context) {
	storage := NewMemStorage()
	for i := 0; i < 100; i++ {
		storage.UpdateGauge(ctx, "test", float64(i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.GetAll()
	}
}
