package main

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func computeHMACSHA256(data, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
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

func sendMetricJSON(client *http.Client, name, metricType string, serverAddr string, value *float64, delta *int64, key []byte) error {
	var b bytes.Buffer

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

	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return fmt.Errorf("failed to compress body: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compBody := b.Bytes()

	var hashHeader string
	if len(key) > 0 {
		hashHeader = computeHMACSHA256(compBody, key)
	}

	url := fmt.Sprintf("http://%s/update", serverAddr)
	req, err := http.NewRequest(http.MethodPost, url, &b)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if hashHeader != "" {
		req.Header.Set("HashSHA256", hashHeader)
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

func sendBatchJSON(client *http.Client, metricsList []Metrics, serverAddr string, key []byte) error {
	if len(metricsList) == 0 {
		return nil
	}

	body, err := json.Marshal(metricsList)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return fmt.Errorf("failed to compress batch: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compressedBody := b.Bytes()

	var hashHeader string
	if len(key) > 0 {
		hashHeader = computeHMACSHA256(compressedBody, key)
	}

	url := fmt.Sprintf("http://%s/updates", serverAddr)
	req, err := http.NewRequest(http.MethodPost, url, &b)
	if err != nil {
		return fmt.Errorf("failed to create batch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if hashHeader != "" {
		req.Header.Set("HashSHA256", hashHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
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
