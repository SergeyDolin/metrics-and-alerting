package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

type (
	responseData struct {
		status int
		size   int
	}

	loggingResponseWriter struct {
		http.ResponseWriter
		responseData *responseData
	}
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.Header().Set("Vary", "Accept-Encoding")
	w.ResponseWriter.WriteHeader(statusCode)
}

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

func logMiddleware(logger *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			responseData := &responseData{
				status: 0,
				size:   0,
			}

			lw := loggingResponseWriter{
				ResponseWriter: w,
				responseData:   responseData,
			}

			defer func() {
				if err := recover(); err != nil {
					logger.Errorf("PANIC recovered: %v", err)
					http.Error(&lw, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(&lw, r)
			duration := time.Since(start)
			logger.Infof("%s %s %d %v %d", r.RequestURI, r.Method, responseData.status, duration, responseData.size)
		})
	}
}

func isCompressible(contentType string) bool {
	if contentType == "" {
		return false
	}
	parts := strings.SplitN(strings.ToLower(contentType), ";", 2)
	mimeType := strings.TrimSpace(parts[0])
	return mimeType == "text/html" || mimeType == "application/json"
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Распаковка входящего тела, если Content-Encoding: gzip
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			r.Body = gz
			r.Header.Del("Content-Length") // важно при замене тела
		}

		// Если клиент не поддерживает gzip — пропускаем
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Оборачиваем в специальный writer, который примет решение о сжатии
		// после того, как обработчик установит Content-Type
		gzipWrapper := &conditionalGzipResponseWriter{
			ResponseWriter: w,
			supportsGzip:   true,
		}

		next.ServeHTTP(gzipWrapper, r)

		// Закрываем gzip.Writer, если он был создан
		if gzipWrapper.gz != nil {
			gzipWrapper.gz.Close()
		}
	})
}

type conditionalGzipResponseWriter struct {
	http.ResponseWriter
	gz           *gzip.Writer
	supportsGzip bool
	wroteHeader  bool
}

func (w *conditionalGzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *conditionalGzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	contentType := w.Header().Get("Content-Type")
	if w.supportsGzip && isCompressible(contentType) {

		w.gz = gzip.NewWriter(w.ResponseWriter)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.ResponseWriter.WriteHeader(statusCode)
	} else {

		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func verifySignatureMiddleware(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(key) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			r.Body = io.NopCloser(bytes.NewReader(body))

			receivedHash := r.Header.Get("HashSHA256")
			if receivedHash == "" {
				http.Error(w, "Missing HashSHA256 header", http.StatusBadRequest)
				return
			}

			expectedHash := computeHMACSHA256(body, key)

			if receivedHash != expectedHash {
				http.Error(w, "Invalid HashSHA256 signature", http.StatusBadRequest)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func signResponseMiddleware(key []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(key) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			recorder := &responseRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(recorder, r)

			responseHash := computeHMACSHA256(recorder.body.Bytes(), key)
			w.Header().Set("HashSHA256", responseHash)

			for k, values := range recorder.Header() {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(recorder.statusCode)
			if recorder.body.Len() > 0 {
				w.Write(recorder.body.Bytes())
			}
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return len(b), nil
}
