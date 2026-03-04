package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/SergeyDolin/metrics-and-alerting/internal/sha256"
)

// responseData holds metadata about the HTTP response for logging purposes.
// It tracks the status code and response size to be logged after request completion.
//
// generate:reset
type responseData struct {
	status int // HTTP status code of the response
	size   int // Size of the response body in bytes
}

// loggingResponseWriter is a wrapper around http.ResponseWriter that captures
// response metadata for logging. It implements the http.ResponseWriter interface
// and intercepts Write and WriteHeader calls to record status code and size.
//
// generate:reset
type loggingResponseWriter struct {
	http.ResponseWriter               // Embedded original ResponseWriter
	responseData        *responseData // Pointer to store response metadata
}

// Write intercepts the Write call to capture the number of bytes written.
// It writes the data to the underlying ResponseWriter and updates the size counter.
//
// Parameters:
//   - b: Byte slice to write to the response
//
// Returns:
//   - int: Number of bytes written
//   - error: Any error encountered during writing
func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

// WriteHeader intercepts the WriteHeader call to capture the status code.
// It sets the status code on the underlying ResponseWriter and stores it.
//
// Parameters:
//   - statusCode: HTTP status code to send
func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

// logMiddleware creates a logging middleware that records detailed information
// about each HTTP request and response. It captures:
//   - Request URI and method
//   - Response status code
//   - Request duration
//   - Response size
//
// The middleware also includes panic recovery to prevent crashes.
//
// Parameters:
//   - logger: Sugared logger for writing log entries
//
// Returns:
//   - func(http.Handler) http.Handler: Middleware function
func logMiddleware(logger *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Initialize response data capture
			responseData := &responseData{
				status: 0,
				size:   0,
			}

			// Wrap the ResponseWriter
			lw := loggingResponseWriter{
				ResponseWriter: w,
				responseData:   responseData,
			}

			// Recover from panics to ensure logging still occurs
			defer func() {
				if err := recover(); err != nil {
					logger.Errorf("PANIC recovered: %v", err)
					http.Error(&lw, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			// Process the request
			next.ServeHTTP(&lw, r)

			// Log request details
			duration := time.Since(start)
			logger.Infof("%s %s %d %v %d", r.RequestURI, r.Method, responseData.status, duration, responseData.size)
		})
	}
}

// isCompressible determines if a content type should be compressed with gzip.
// Compressible types are text/html and application/json.
//
// Parameters:
//   - contentType: The Content-Type header value
//
// Returns:
//   - bool: true if the content type should be compressed
func isCompressible(contentType string) bool {
	if contentType == "" {
		return false
	}
	// Extract MIME type without parameters (e.g., "text/html; charset=utf-8" -> "text/html")
	parts := strings.SplitN(strings.ToLower(contentType), ";", 2)
	mimeType := strings.TrimSpace(parts[0])
	return mimeType == "text/html" || mimeType == "application/json"
}

// gzipMiddleware handles both decompression of gzipped requests and
// conditional compression of responses. It:
//  1. Decompresses request body if Content-Encoding: gzip is present
//  2. Compresses response body if client accepts gzip and content is compressible
//
// The middleware uses conditionalGzipResponseWriter to delay compression
// decision until the Content-Type is known.
//
// Returns:
//   - func(http.Handler) http.Handler: Middleware function
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decompress gzipped request body if present
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			r.Body = gz
			r.Header.Del("Content-Length") // Remove Content-Length as body size changed
		}

		// Skip response compression if client doesn't support gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Wrap response writer to conditionally compress
		gzipWrapper := &conditionalGzipResponseWriter{
			ResponseWriter: w,
			supportsGzip:   true,
		}

		next.ServeHTTP(gzipWrapper, r)

		// Close gzip writer if it was created
		if gzipWrapper.gz != nil {
			gzipWrapper.gz.Close()
		}
	})
}

// conditionalGzipResponseWriter is a wrapper that conditionally compresses
// responses with gzip. It waits until WriteHeader is called to determine
// the Content-Type and decide whether compression is appropriate.
//
// generate:reset
type conditionalGzipResponseWriter struct {
	http.ResponseWriter              // Embedded original ResponseWriter
	gz                  *gzip.Writer // Gzip writer, created only if compression is used
	supportsGzip        bool         // Whether client supports gzip
	wroteHeader         bool         // Flag to prevent multiple WriteHeader calls
}

// Write implements the http.ResponseWriter interface. If compression is enabled,
// it writes through the gzip writer; otherwise, it writes directly.
//
// Parameters:
//   - b: Byte slice to write
//
// Returns:
//   - int: Number of bytes written
//   - error: Any error encountered
func (w *conditionalGzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// WriteHeader implements the http.ResponseWriter interface. It makes the final
// decision about compression based on the Content-Type header and client support.
// If compression is appropriate, it initializes the gzip writer and sets
// appropriate headers before writing the status code.
//
// Parameters:
//   - statusCode: HTTP status code to send
func (w *conditionalGzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	// Determine if response should be compressed
	contentType := w.Header().Get("Content-Type")
	if w.supportsGzip && isCompressible(contentType) {
		// Initialize gzip compression
		w.gz = gzip.NewWriter(w.ResponseWriter)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding") // Indicate that response varies on Accept-Encoding
		w.ResponseWriter.WriteHeader(statusCode)
	} else {
		// No compression
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

// hashVerificationMiddleware verifies HMAC-SHA256 signatures on incoming requests.
// If a key is configured (flagKey != ""), it:
//  1. Reads the entire request body
//  2. Verifies the hash provided in the HashSHA256 header matches the body
//  3. Replaces the request body with a fresh reader for downstream handlers
//
// If verification fails, it returns HTTP 400 Bad Request.
//
// Returns:
//   - func(http.Handler) http.Handler: Middleware function
func hashVerificationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only verify if a key is configured
		if flagKey != "" {
			// Read the entire request body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}

			// Verify the hash
			expectedHash := r.Header.Get("HashSHA256")
			if !sha256.VerifyHashSHA256(body, flagKey, expectedHash) {
				http.Error(w, "Hash verification failed", http.StatusBadRequest)
				return
			}

			// Replace the request body with a fresh reader for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		// Continue to the next handler
		next.ServeHTTP(w, r)
	})
}
