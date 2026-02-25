package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/SergeyDolin/metrics-and-alerting/internal/sha256"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func sendMetric(client *http.Client, name, typeMetric string, value string, serverAddr string) error {
	// http://<АДРЕС_СЕРВЕРА>/update/<ТИП_МЕТРИКИ>/<ИМЯ_МЕТРИКИ>/<ЗНАЧЕНИЕ_МЕТРИКИ>
	url := fmt.Sprintf("http://%s/update/%s/%s/%s", serverAddr, typeMetric, name, value)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("response error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server return code %d", resp.StatusCode)
	}
	return nil
}

func sendRequest(client *http.Client, url string, body []byte) error {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	gz := gzip.NewWriter(buf)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return fmt.Errorf("failed to compress body: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	if *key != "" {
		hash := sha256.ComputeHMACSHA256(body, *key)
		req.Header.Set("HashSHA256", hash)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return &httpError{
			statusCode: resp.StatusCode,
			msg:        string(bodyBytes),
		}
	}
	return nil
}

func sendMetricJSON(client *http.Client, name, metricType string, serverAddr string, value *float64, delta *int64) error {
	metric := Metrics{
		ID:    name,
		MType: metricType,
		Value: value,
		Delta: delta,
	}

	body, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("failed to marshal metric: %w", err)
	}

	url := fmt.Sprintf("http://%s/update", serverAddr)
	return sendRequest(client, url, body)
}

func sendBatchJSON(client *http.Client, metricsList []Metrics, serverAddr string) error {

	if len(metricsList) == 0 {
		return nil
	}

	body, err := json.Marshal(metricsList)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	url := fmt.Sprintf("http://%s/updates", serverAddr)

	return sendRequest(client, url, body)
}
