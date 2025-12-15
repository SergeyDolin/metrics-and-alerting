package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	client := http.Client{}

	parseArgs()

	queue := NewMetricQueue(100)

	pool := NewWorkerPool(*rateLimit, queue, &client, *sAddr)
	pool.Start()

	go func() {
		pollInterval := time.Duration(*pInterval) * time.Second
		counter := int64(0)

		for {
			time.Sleep(pollInterval)

			runtimeMetrics := make(map[string]float64)
			CollectRuntimeMetrics(runtimeMetrics)

			systemMetrics := make(map[string]float64)
			CollectSystemMetrics(systemMetrics)

			counter++

			for name, value := range runtimeMetrics {
				queue.Push(Metrics{
					ID:    name,
					MType: "gauge",
					Value: &value,
				})
			}

			for name, value := range systemMetrics {
				queue.Push(Metrics{
					ID:    name,
					MType: "gauge",
					Value: &value,
				})
			}

			queue.Push(Metrics{
				ID:    "PollCount",
				MType: "counter",
				Delta: &counter,
			})
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Printf("Shutdown signal received, waiting for workers to finish.")

	pool.Stop()

	time.Sleep(time.Duration(*pInterval) * time.Second)
	for !queue.IsEmpty() {
		metric := queue.Pop()
		sendMetricJSON(&client, metric.ID, metric.MType, *sAddr, metric.Value, metric.Delta)
	}

	log.Println("Agent shutdown complete.")
}
