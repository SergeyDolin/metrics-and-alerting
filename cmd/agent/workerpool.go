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

type WorkerPool struct {
	workers    int
	queue      *MetricQueue
	client     *http.Client
	serverAddr string
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewWorkerPool(workers int, queue *MetricQueue, client *http.Client, serverAddr string) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:    workers,
		queue:      queue,
		client:     client,
		serverAddr: serverAddr,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

func (wp *WorkerPool) Stop() {
	wp.cancel()
	wp.wg.Wait()
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case metric := <-wp.queue.queue:
			err := retryWithBackoff(func() error {
				if metric.MType == "gauge" {
					return sendMetricJSON(wp.client, metric.ID, metric.MType, wp.serverAddr, wp.queue.Pop().Value, nil)
				} else {
					return sendMetricJSON(wp.client, metric.ID, metric.MType, wp.serverAddr, nil, wp.queue.Pop().Delta)
				}
			})
			if err != nil {
				log.Printf("Worker %d: Failed to send metric %s: %v\n", id, metric.ID, err)
			}
		}
	}
}

func CollectRuntimeMetrics(ms map[string]float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

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

	for name, metric := range fieldMap {
		ms[name] = metric(&m)
	}

	ms["RandomValue"] = rand.Float64()
}

func CollectSystemMetrics(ms map[string]float64) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error getting memory stats: %v\n", err)
	} else {
		ms["TotalMemory"] = float64(vmStat.Total)
		ms["FreeMemory"] = float64(vmStat.Free)
	}

	cpuPercentages, err := cpu.Percent(time.Second, true)
	if err != nil {
		log.Printf("Error getting CPU stats: %v\n", err)
	} else {
		for i, percent := range cpuPercentages {
			ms[fmt.Sprintf("CPUutilization%d", i+1)] = percent
		}
	}
}
