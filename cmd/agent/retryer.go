package main

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

// httpError represents a custom error type for HTTP-related failures.
// It encapsulates both the HTTP status code and the response body message
// to provide detailed context about why a request failed.
//
// generate:reset
type httpError struct {
	statusCode int    // HTTP status code returned by the server (e.g., 500, 502, 404)
	msg        string // Response body content for additional error details
}

// Error implements the error interface for httpError.
// Returns a formatted string containing the status code and response message.
func (e *httpError) Error() string {
	return fmt.Sprintf("server returned status %d: %s", e.statusCode, e.msg)
}

// isRetriableHTTPError determines if an HTTP status code indicates a temporary
// server-side issue that might be resolved by retrying the request.
//
// Returns true for:
//   - 5xx Server Error status codes (500-599) which indicate server-side issues
//
// Returns false for:
//   - 4xx Client Error status codes (400-499) which indicate client-side issues that won't be resolved by retrying
//   - 2xx Success status codes (200-299) - though these shouldn't be errors in the first place
//   - 3xx Redirection status codes (300-399) - these require different handling
func isRetriableHTTPError(statusCode int) bool {
	return statusCode >= 500 && statusCode <= 599
}

// isRetriableNetworkError examines an error to determine if it represents
// a temporary network issue that might be resolved by retrying the operation.
//
// It recursively unwraps *url.Error to examine the underlying error and checks for:
//   - Network operation errors (*net.OpError)
//   - Timeout errors (net.Error with Timeout() true)
//
// Parameters:
//   - err: The error to examine, which may be nil or various network-related error types
//
// Returns:
//   - bool: true if the error indicates a temporary network issue that can be retried,
//     false if the error is nil or not retriable
func isRetriableNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Unwrap URL errors to examine the underlying cause
	switch e := err.(type) {
	case *url.Error:
		return isRetriableNetworkError(e.Err)
	case *net.OpError:
		// Network operation errors (connection refused, timeout, etc.) are typically retriable
		return true
	case net.Error:
		// Check if it's a timeout error (can be retried)
		return e.Timeout()
	}
	// For any other error types, assume they might be retriable
	return true
}

// retryWithBackoff executes a function with exponential backoff retry logic.
// It attempts to execute the provided function up to 4 times with increasing delays
// between attempts (0s, 1s, 3s, 5s) when encountering retriable errors.
//
// The function considers two types of errors as retriable:
//   - Network errors (timeouts, connection refused, etc.)
//   - HTTP 5xx Server Error status codes
//
// Parameters:
//   - fn: The function to execute, which returns an error if unsuccessful
//
// Returns:
//   - error: nil if the function succeeds, or an error if:
//   - All retry attempts fail with retriable errors
//   - A non-retriable error occurs (4xx HTTP error, etc.)
//
// The backoff strategy:
//   - Attempt 1: No delay (immediate retry)
//   - Attempt 2: 1 second delay
//   - Attempt 3: 3 second delay
//   - Attempt 4: 5 second delay
func retryWithBackoff(fn func() error) error {
	// Define retry delays: first attempt immediate, then increasing backoff
	delays := []time.Duration{0, 1 * time.Second, 3 * time.Second, 5 * time.Second}

	// Iterate through each retry attempt
	for i, delay := range delays {
		// Apply delay before this attempt (except for first attempt with delay=0)
		if delay > 0 {
			time.Sleep(delay)
		}

		// Execute the provided function
		err := fn()
		if err == nil {
			return nil // Success!
		}

		// Check if the error is retriable
		retriable := false

		// Check for retriable network errors
		if netErr := isRetriableNetworkError(err); netErr {
			retriable = true
		}

		// Check for retriable HTTP errors (5xx status codes)
		if httpErr, ok := err.(*httpError); ok {
			if isRetriableHTTPError(httpErr.statusCode) {
				retriable = true
			}
		}

		// If error is not retriable, fail immediately
		if !retriable {
			return err
		}

		// If this was the last attempt and still failing, return an error
		if i == len(delays)-1 {
			return fmt.Errorf("failed after %d attempts: %w", len(delays), err)
		}

		// Otherwise, continue to next retry attempt
	}
	return nil
}
