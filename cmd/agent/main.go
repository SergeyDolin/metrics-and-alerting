package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "net/http/pprof" // Import for side effects: enables pprof profiling endpoints

	"go.uber.org/zap"
)

// Build information variables - set during compilation with ldflags
var (
	// buildVersion contains the version of the build (e.g., "1.0.0")
	buildVersion string = "N/A"

	// buildDate contains the date when the build was created (e.g., "2025-02-27T10:00:00Z")
	buildDate string = "N/A"

	// buildCommit contains the git commit hash of the source code
	buildCommit string = "N/A"
)

// printBuildInfo displays build version information on application startup.
func printBuildInfo() {
	fmt.Printf("Build version: %s\n", buildVersion)
	fmt.Printf("Build date: %s\n", buildDate)
	fmt.Printf("Build commit: %s\n", buildCommit)
	fmt.Println()
}

// main is the entry point for the metrics collection agent.
// It initializes and runs the agent that periodically collects system and runtime metrics,
// queues them for processing, and sends them to a monitoring server.
//
// The agent performs the following key functions:
//  1. Starts a pprof profiling server for debugging and performance analysis
//  2. Parses configuration from command-line flags and environment variables
//  3. Initializes a metric queue and worker pool for concurrent metric processing
//  4. Runs a metric collection loop that gathers runtime and system metrics
//  5. Handles graceful shutdown on SIGINT and SIGTERM signals
//
// The agent continues running until it receives a termination signal,
// at which point it ensures all queued metrics are processed before exiting.
func main() {
	// Initialize structured logger for the application
	logger, err := zap.NewDevelopment()
	if err != nil {
		logger.Fatal("cannot initialize zap")
	}
	defer logger.Sync() // Flush any buffered log entries
	log := logger.Sugar()
	// Print build information on startup for debugging and traceability
	printBuildInfo()
	// Start pprof profiling server in a separate goroutine
	// This provides performance profiling endpoints at http://localhost:8081/debug/pprof/
	go func() {
		log.Infoln("pprof server started on :8081")
		log.Infoln(http.ListenAndServe("localhost:8081", nil))
	}()

	// Initialize HTTP client for sending metrics to the server
	client := http.Client{}

	// Parse configuration from flags and environment variables
	parseArgs()

	// Create a buffered queue for metrics with capacity of 100 items
	// This queue acts as a buffer between metric collection and sending
	queue := NewMetricQueue(100)

	// Create and start a worker pool for concurrent metric processing
	// The pool size is determined by the rateLimit configuration
	pool := NewWorkerPool(*rateLimit, queue, &client, *sAddr)
	pool.Start()

	// Start metric collection goroutine
	// This loop runs continuously, collecting metrics at the specified poll interval
	go func() {
		// Convert poll interval from seconds to time.Duration
		pollInterval := time.Duration(*pInterval) * time.Second

		// Initialize counter for PollCount metric
		counter := int64(0)

		// Pre-allocate maps for metrics to reduce allocations during collection
		runtimeMetrics := make(map[string]float64, 30) // Runtime metrics like MemStats
		systemMetrics := make(map[string]float64, 5)   // System metrics like CPU, memory

		// Infinite collection loop
		for {
			// Wait for the next poll interval
			time.Sleep(pollInterval)

			// Collect Go runtime metrics (MemStats, etc.)
			CollectRuntimeMetrics(runtimeMetrics)

			// Collect system-level metrics (CPU, memory usage, etc.)
			CollectSystemMetrics(systemMetrics)

			// Increment PollCount counter for each collection cycle
			counter++

			// Queue all runtime gauge metrics for sending
			for name, value := range runtimeMetrics {
				queue.Push(Metrics{
					ID:    name,
					MType: "gauge",
					Value: &value,
				})
			}

			// Queue all system gauge metrics for sending
			for name, value := range systemMetrics {
				queue.Push(Metrics{
					ID:    name,
					MType: "gauge",
					Value: &value,
				})
			}

			// Queue the PollCount counter metric
			queue.Push(Metrics{
				ID:    "PollCount",
				MType: "counter",
				Delta: &counter,
			})
		}
	}()

	// Set up signal handling for graceful shutdown
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	// Notify the channel on SIGINT (Ctrl+C), SIGTERM (termination signal), and SIGQUIT
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Block until a signal is received
	<-sigChan
	log.Infoln("Shutdown signal received, waiting for workers to finish.")

	// Stop the worker pool - no new metrics will be processed
	pool.Stop()

	// Allow time for in-flight requests to complete
	time.Sleep(time.Duration(*pInterval) * time.Second)

	// Process any remaining metrics in the queue synchronously
	// This ensures no metrics are lost during shutdown
	for !queue.IsEmpty() {
		metric := queue.Pop()
		// Send each remaining metric directly (bypassing the worker pool)
		sendMetricJSON(&client, metric.ID, metric.MType, *sAddr, metric.Value, metric.Delta)
	}

	// Log completion and exit
	log.Infoln("Agent shutdown complete.")
}
