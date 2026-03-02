package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/SergeyDolin/metrics-and-alerting/internal/crypto"
	"github.com/SergeyDolin/metrics-and-alerting/internal/sha256"
)

// bufferPool is a sync.Pool for reusing bytes.Buffer instances to reduce memory allocations
// when compressing request bodies with gzip. This improves performance by avoiding
// repeated buffer creation and garbage collection overhead.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// sendMetric sends a single metric to the server using the URL path format (deprecated method).
// It constructs a URL in the format: http://<serverAddr>/update/<typeMetric>/<name>/<value>
// and sends a POST request with Content-Type: text/plain.
//
// Parameters:
//   - client: HTTP client used to send the request
//   - name: Name of the metric
//   - typeMetric: Type of the metric (e.g., "gauge", "counter")
//   - value: String representation of the metric value
//   - serverAddr: Server address in "host:port" format
//
// Returns:
//   - error: nil if successful, otherwise an error describing what went wrong
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

// sendRequest is a helper function that sends an HTTP POST request with gzip compression.
// It compresses the provided body using gzip, adds appropriate headers, and includes
// a HMAC-SHA256 hash if a secret key is configured.
//
// Parameters:
//   - client: HTTP client used to send the request
//   - url: Target URL for the request
//   - body: JSON-encoded body to be compressed and sent
//
// Returns:
//   - error: nil if successful (HTTP 200 OK), otherwise an error with details
//     including the server's response body for non-200 status codes
func sendRequest(client *http.Client, url string, body []byte) error {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// Encrypt the body if crypto key is provided
	var reqBody []byte
	if *cryptoKey != "" {
		publicKey, err := crypto.LoadRSAPublicKey(*cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to load public key: %w", err)
		}

		encryptedBody, err := crypto.EncryptWithPublicKey(publicKey, body)
		if err != nil {
			return fmt.Errorf("failed to encrypt request body: %w", err)
		}

		reqBody = encryptedBody
	} else {
		reqBody = body
	}

	gz := gzip.NewWriter(buf)
	if _, err := gz.Write(reqBody); err != nil {
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

// sendMetricJSON sends a single metric to the server in JSON format.
// It creates a Metrics struct with the provided parameters, marshals it to JSON,
// and sends it using the sendRequest helper function.
//
// Parameters:
//   - client: HTTP client used to send the request
//   - name: Name of the metric
//   - metricType: Type of the metric ("gauge" or "counter")
//   - serverAddr: Server address in "host:port" format
//   - value: Pointer to float64 value (used for gauge metrics)
//   - delta: Pointer to int64 value (used for counter metrics)
//
// Returns:
//   - error: nil if successful, otherwise an error from sendRequest or JSON marshaling
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

// sendBatchJSON sends a batch of metrics to the server in a single request.
// It marshals the entire slice of Metrics to JSON and sends it to the /updates endpoint.
// This is more efficient than sending multiple individual requests when dealing with
// multiple metrics.
//
// Parameters:
//   - client: HTTP client used to send the request
//   - metricsList: Slice of Metrics structs to be sent
//   - serverAddr: Server address in "host:port" format
//
// Returns:
//   - error: nil if successful or if the metricsList is empty,
//     otherwise an error from sendRequest or JSON marshaling
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
