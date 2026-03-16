package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/SergeyDolin/metrics-and-alerting/internal/proto"
	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
	"github.com/go-chi/chi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.uber.org/zap"

	"github.com/go-chi/chi/middleware"
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

// main is the entry point for the metrics collection server application.
// It initializes the chi router, creates the appropriate metrics storage backend,
// configures HTTP routes, and starts the server.
//
// The server supports multiple storage backends:
//   - PostgreSQL database (when DATABASE_DSN is provided)
//   - File-based storage (when FILE_STORAGE_PATH is provided)
//   - In-memory storage (fallback)
//
// The following endpoints are configured:
//   - GET / - HTML page listing all metrics
//   - POST /update - Update a single metric via JSON
//   - POST /updates - Batch update multiple metrics via JSON
//   - GET /ping - Database health check (if using PostgreSQL)
//   - POST /value - Retrieve a metric value via JSON
//   - POST /update/{type}/{name}/{value} - Update a metric via URL parameters (legacy)
//   - GET /value/{type}/{name} - Retrieve a metric value via URL parameters (legacy)
//
// The server also supports:
//   - Gzip compression middleware
//   - HMAC signature verification when a key is configured
//   - Request logging
//   - Audit logging to file or HTTP endpoint when configured
//   - Periodic or synchronous metric persistence to disk
func main() {
	// Print build information on startup for debugging and traceability
	printBuildInfo()
	// Parse configuration from command-line flags and environment variables
	parseFlags()

	// Initialize structured logger for the application
	logger, err := zap.NewDevelopment()
	if err != nil {
		logger.Fatal("cannot initialize zap")
	}
	defer logger.Sync() // Flush any buffered log entries
	sugar := logger.Sugar()

	// Initialize router
	router := chi.NewRouter()

	// Storage interface and save function for metrics persistence
	var store storage.Storage
	var saveSync func()

	// Configure storage backend based on flags
	if flagSQL != "" {
		// PostgreSQL database storage
		sugar.Infof("Initializing PostgreSQL storage with DSN: %s", flagSQL)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dbStorage, err := storage.NewDBStorage(ctx, flagSQL)
		if err != nil {
			sugar.Fatalf("Failed to open DB connection: %v", err)
		}
		store = dbStorage
		// Ensure database connection is properly closed on exit
		defer func() {
			cleanupCtx := context.Background()
			if err := dbStorage.SaveAll(cleanupCtx); err != nil {
				sugar.Errorf("Failed to save metrics on exit: %v", err)
			}
			dbStorage.Close()
		}()

		// Database storage persists immediately, no sync function needed
		saveSync = func() {}
	} else {
		// File-based or in-memory storage
		if flagFileStoragePath != "" && flagRestore {
			// Attempt to restore metrics from file if configured
			fileStorage, err := storage.NewFileStorage(flagFileStoragePath)
			if err != nil {
				sugar.Warnf("Failed to restore metrics from file: %v", err)
			} else {
				store = fileStorage
			}
		}

		// Fallback to in-memory storage if no file storage was created
		if store == nil {
			store = storage.NewMemStorage()
		}

		// Configure periodic background saving for file storage
		if flagStoreInterval > 0 && flagFileStoragePath != "" {
			if fs, ok := store.(*storage.FileStorage); ok {
				go func() {
					ticker := time.NewTicker(flagStoreInterval)
					defer ticker.Stop()
					for range ticker.C {
						if err := fs.Save(); err != nil {
							sugar.Errorf("Periodic save failed: %v", err)
						}
					}
				}()
			}
		}

		// Configure synchronous save function for file storage
		// This is used when flagStoreInterval == 0 (save after each update)
		saveSync = func() {
			if flagStoreInterval == 0 && flagFileStoragePath != "" {
				if fs, ok := store.(*storage.FileStorage); ok {
					if err := fs.Save(); err != nil {
						sugar.Errorf("Sync save failed: %v", err)
					}
				}
			}
		}
	}

	// Configure audit logging observers
	var auditObservers []Observer
	if flagAuditFile != "" {
		// Add file-based audit logging
		auditObservers = append(auditObservers, NewFileWriterObserver(flagAuditFile))
	}
	if flagAuditURL != "" {
		// Add HTTP-based audit logging
		auditObservers = append(auditObservers, NewHTTPSenderObserver(flagAuditURL))
	}

	// Create audit publisher if any observers are configured
	var auditPublisher *Publisher
	if len(auditObservers) > 0 {
		auditPublisher = NewPublisher(auditObservers)
		defer auditPublisher.Close() // Ensure all audit logs are flushed on shutdown
	}

	// Start gRPC server if address is configured
	if flagGRPCAddr != "" {
		go startGRPCServer(flagGRPCAddr, store, saveSync, auditPublisher, flagTrustedSubnet)
	}

	// Apply global middleware to all routes
	router.Use(middleware.StripSlashes) // Remove trailing slashes from URLs
	router.Use(gzipMiddleware)          // Support gzip compression for requests/responses
	if flagTrustedSubnet != "" {
		// Add trusted subnet validation middleware if configured
		router.Use(trustedSubnetMiddleware)
	}
	if flagKey != "" {
		// Add HMAC signature verification middleware if key is configured
		router.Use(hashVerificationMiddleware)
	}
	router.Use(logMiddleware(sugar)) // Add request logging

	// Initialize all handler functions
	indexHandlerFunc := indexHandler(store)
	updateJSONHandlerFunc := updateJSONHandler(context.Background(), store, saveSync, auditPublisher)
	updatesBatchHandlerFunc := updatesBatchHandler(context.Background(), store, saveSync, auditPublisher)
	valueJSONHandlerFunc := valueJSONHandler(store, auditPublisher)
	postHandlerFunc := postHandler(context.Background(), store, saveSync, auditPublisher)
	getHandlerFunc := getHandler(store, auditPublisher)
	pingSQLHandlerFunc := pingSQLHandler(store)

	// Configure custom error handlers
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		// Return 405 for methods not allowed on a route
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for non-existent routes
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	// Register routes with their handlers
	router.Get("/", indexHandlerFunc)                             // HTML metrics listing
	router.Post("/update", updateJSONHandlerFunc)                 // Single metric JSON update
	router.Post("/updates", updatesBatchHandlerFunc)              // Batch JSON update
	router.Get("/ping", pingSQLHandlerFunc)                       // Database health check
	router.Post("/value", valueJSONHandlerFunc)                   // JSON metric retrieval
	router.Post("/update/{type}/{name}/{value}", postHandlerFunc) // Legacy URL param update
	router.Get("/value/{type}/{name}", getHandlerFunc)            // Legacy URL param retrieval

	// Create a context that will be canceled when a shutdown signal is received
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	// Start the HTTP server in a goroutine
	srv := &http.Server{
		Addr:    flagRunAddr,
		Handler: router,
	}
	go func() {
		sugar.Infof("Running server on %s", flagRunAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sugar.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Block until a signal is received (context is canceled)
	<-ctx.Done()
	sugar.Info("Shutdown signal received")

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt to save all metrics before shutting down
	if store != nil {
		saveSync()
	}

	// Shutdown the server gracefully
	if err := srv.Shutdown(shutdownCtx); err != nil {
		sugar.Errorf("Server shutdown error: %v", err)
	} else {
		sugar.Info("Server shutdown complete")
	}

	// Start the HTTP server
	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))
}

// startGRPCServer starts the gRPC server
func startGRPCServer(addr string, store storage.Storage, saveFunc func(), auditPublisher *Publisher, trustedSubnet string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen for gRPC: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(subnetInterceptor(trustedSubnet)),
	)

	metricsServer := NewGRPCServer(store, saveFunc, auditPublisher, trustedSubnet)
	proto.RegisterMetricsServer(grpcServer, metricsServer)

	log.Printf("Starting gRPC server on %s", addr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}

// subnetInterceptor creates a unary interceptor for trusted subnet validation
func subnetInterceptor(trustedSubnet string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if trustedSubnet == "" {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.PermissionDenied, "metadata not found")
		}

		// Get client IP from metadata
		var clientIP string
		if values := md.Get("x-real-ip"); len(values) > 0 {
			clientIP = values[0]
		}

		if clientIP == "" {
			return nil, status.Error(codes.PermissionDenied, "client IP not found in metadata")
		}

		// Parse trusted subnet
		_, ipNet, err := net.ParseCIDR(trustedSubnet)
		if err != nil {
			return nil, status.Error(codes.Internal, "invalid trusted subnet configuration")
		}

		// Parse client IP
		ip := net.ParseIP(clientIP)
		if ip == nil {
			return nil, status.Error(codes.InvalidArgument, "invalid IP address format")
		}

		// Check if IP is in trusted subnet
		if !ipNet.Contains(ip) {
			return nil, status.Error(codes.PermissionDenied, "IP address not in trusted subnet")
		}

		return handler(ctx, req)
	}
}
