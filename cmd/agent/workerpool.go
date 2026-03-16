package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// WorkerPool manages a pool of worker goroutines that process and send metrics
// from a queue to the monitoring server. It provides controlled concurrency
// with graceful shutdown capabilities.
//
// generate:reset
type WorkerPool struct {
	workers    int
	queue      *MetricQueue
	client     *http.Client
	grpcClient *GRPCClient
	serverAddr string
	useGRPC    bool
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWorkerPool creates and initializes a new WorkerPool with the specified parameters.
//
// Parameters:
//   - workers: Number of concurrent worker goroutines to start
//   - queue: Pointer to the MetricQueue containing pending metrics
//   - client: HTTP client for making requests to the server
//   - serverAddr: Address of the monitoring server in "host:port" format
//
// Returns:
//   - *WorkerPool: A configured worker pool ready to be started
func NewWorkerPool(workers int, queue *MetricQueue, client *http.Client, grpcClient *GRPCClient, serverAddr string) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:    workers,
		queue:      queue,
		client:     client,
		grpcClient: grpcClient,
		serverAddr: serverAddr,
		useGRPC:    grpcClient != nil,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start launches the specified number of worker goroutines.
// Each worker runs concurrently and processes metrics from the queue
// until Stop() is called or the context is cancelled.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Stop signals all workers to shut down gracefully and waits for them to finish.
// It cancels the context, which causes workers to exit their processing loops,
// then waits for all workers to complete using WaitGroup.
func (wp *WorkerPool) Stop() {
	wp.cancel()
	wp.wg.Wait()
}

// worker is the main processing loop for individual worker goroutines.
// It continuously retrieves metrics from the queue and processes them
// until shutdown is signaled via context cancellation or the queue is closed.
//
// Parameters:
//   - id: Unique identifier for this worker (used for logging)
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			// Shutdown signal received - exit gracefully
			return
		case metric, ok := <-wp.queue.queue:
			if !ok {
				// Queue channel was closed - exit
				return
			}
			// Process the retrieved metric
			wp.processMetric(id, &metric)
		}
	}
}

// processMetric handles the sending of a single metric to the server.
// It uses the retryWithBackoff function to handle transient failures
// and returns the metric object to the pool after processing.
//
// Parameters:
//   - id: Worker ID for logging purposes
//   - metric: Pointer to the Metrics object to be sent
func (wp *WorkerPool) processMetric(id int, metric *Metrics) {
	defer wp.queue.pool.Put(metric)

	var err error
	if wp.useGRPC {
		err = retryWithBackoff(func() error {
			return wp.grpcClient.SendMetricsBatch([]Metrics{*metric})
		})
	} else {
		err = retryWithBackoff(func() error {
			if metric.MType == "gauge" {
				return sendMetricJSON(wp.client, metric.ID, metric.MType, wp.serverAddr, metric.Value, nil)
			} else {
				return sendMetricJSON(wp.client, metric.ID, metric.MType, wp.serverAddr, nil, metric.Delta)
			}
		})
	}

	if err != nil {
		log.Printf("Worker %d: Failed to send metric %s: %v\n", id, metric.ID, err)
	}
}

// CollectRuntimeMetrics gathers Go runtime memory statistics and populates
// the provided map with various metrics from runtime.MemStats.
//
// The function collects:
//   - All fields from runtime.MemStats (Alloc, HeapAlloc, GC stats, etc.)
//   - A random value (RandomValue) for testing/demonstration purposes
//
// Parameters:
//   - ms: A map that will be populated with metric names as keys and float64 values
//
// Note: The map should be pre-allocated for better performance
func CollectRuntimeMetrics(ms map[string]float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Map of metric names to extractor functions
	// Each function extracts a specific field from MemStats and converts it to float64
	fieldMap := map[string]func(*runtime.MemStats) float64{
		"Alloc":         func(m *runtime.MemStats) float64 { return float64(m.Alloc) },
		"BuckHashSys":   func(m *runtime.MemStats) float64 { return float64(m.BuckHashSys) },
		"Frees":         func(m *runtime.MemStats) float64 { return float64(m.Frees) },
		"GCCPUFraction": func(m *runtime.MemStats) float64 { return float64(m.GCCPUFraction) },
		"GCSys":         func(m *runtime.MemStats) float64 { return float64(m.GCSys) },
		"HeapAlloc":     func(m *runtime.MemStats) float64 { return float64(m.HeapAlloc) },
		"HeapIdle":      func(m *runtime.MemStats) float64 { return float64(m.HeapIdle) },
		"HeapObjects":   func(m *runtime.MemStats) float64 { return float64(m.HeapObjects) },
		"HeapReleased":  func(m *runtime.MemStats) float64 { return float64(m.HeapReleased) },
		"HeapSys":       func(m *runtime.MemStats) float64 { return float64(m.HeapSys) },
		"LastGC":        func(m *runtime.MemStats) float64 { return float64(m.LastGC) },
		"Lookups":       func(m *runtime.MemStats) float64 { return float64(m.Lookups) },
		"MCacheInuse":   func(m *runtime.MemStats) float64 { return float64(m.MCacheInuse) },
		"MCacheSys":     func(m *runtime.MemStats) float64 { return float64(m.MCacheSys) },
		"MSpanSys":      func(m *runtime.MemStats) float64 { return float64(m.MSpanSys) },
		"Mallocs":       func(m *runtime.MemStats) float64 { return float64(m.Mallocs) },
		"NextGC":        func(m *runtime.MemStats) float64 { return float64(m.NextGC) },
		"NumForcedGC":   func(m *runtime.MemStats) float64 { return float64(m.NumForcedGC) },
		"NumGC":         func(m *runtime.MemStats) float64 { return float64(m.NumGC) },
		"OtherSys":      func(m *runtime.MemStats) float64 { return float64(m.OtherSys) },
		"PauseTotalNs":  func(m *runtime.MemStats) float64 { return float64(m.PauseTotalNs) },
		"StackInuse":    func(m *runtime.MemStats) float64 { return float64(m.StackInuse) },
		"Sys":           func(m *runtime.MemStats) float64 { return float64(m.Sys) },
		"TotalAlloc":    func(m *runtime.MemStats) float64 { return float64(m.TotalAlloc) },
		"HeapInuse":     func(m *runtime.MemStats) float64 { return float64(m.HeapInuse) },
		"MSpanInuse":    func(m *runtime.MemStats) float64 { return float64(m.MSpanInuse) },
		"StackSys":      func(m *runtime.MemStats) float64 { return float64(m.StackSys) },
	}

	// Apply each extractor function to populate the metrics map
	for name, metric := range fieldMap {
		ms[name] = metric(&m)
	}

	// Add a random value for testing/demonstration purposes
	ms["RandomValue"] = rand.Float64()
}

// CollectSystemMetrics gathers system-level metrics using the gopsutil library
// and populates the provided map with:
//   - Memory statistics (TotalMemory, FreeMemory)
//   - CPU utilization per core (CPUutilization1, CPUutilization2, etc.)
//
// Parameters:
//   - ms: A map that will be populated with system metric names as keys and float64 values
//
// The function handles errors gracefully by logging them and continuing,
// allowing partial results to be collected even if some subsystems fail.
func CollectSystemMetrics(ms map[string]float64) {
	// Collect virtual memory statistics
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error getting memory stats: %v\n", err)
	} else {
		ms["TotalMemory"] = float64(vmStat.Total)
		ms["FreeMemory"] = float64(vmStat.Free)
	}

	// Collect CPU utilization percentages per core
	// Parameters: duration to sample (1 second), true = per-cpu percentages
	cpuPercentages, err := cpu.Percent(time.Second, true)
	if err != nil {
		log.Printf("Error getting CPU stats: %v\n", err)
	} else {
		// Add each CPU core's utilization as a separate metric
		for i, percent := range cpuPercentages {
			ms[fmt.Sprintf("CPUutilization%d", i+1)] = percent
		}
	}
}
