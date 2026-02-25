package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkSendMetricJSON(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}
	value := 123.45

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendMetricJSON(client, "test_metric", "gauge", server.URL[7:], &value, nil)
	}
}

func BenchmarkSendBatchJSON(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}
	metricsList := make([]Metrics, 10)
	for i := 0; i < 10; i++ {
		value := float64(i)
		metricsList[i] = Metrics{
			ID:    "test_metric",
			MType: "gauge",
			Value: &value,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendBatchJSON(client, metricsList, server.URL[7:])
	}
}

func BenchmarkCollectRuntimeMetrics(b *testing.B) {
	ms := make(map[string]float64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CollectRuntimeMetrics(ms)
	}
}
