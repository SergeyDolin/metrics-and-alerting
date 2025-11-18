package main

import (
	"fmt"
	"net"
	"net/url"
	"time"
)

type httpError struct {
	statusCode int
	msg        string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("server returned status %d: %s", e.statusCode, e.msg)

}

func isRetriableHTTPError(statusCode int) bool {
	return statusCode >= 500 && statusCode <= 599
}

func isRetriableNetworkError(err error) bool {
	if err == nil {
		return false
	}

	switch e := err.(type) {
	case *url.Error:
		return isRetriableNetworkError(e.Err)
	case *net.OpError:
		return true
	case net.Error:
		return e.Timeout()
	}
	return true
}

func retryWithBackoff(fn func() error) error {
	delays := []time.Duration{0, 1 * time.Second, 3 * time.Second, 5 * time.Second}

	for i, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}

		err := fn()
		if err == nil {
			return nil
		}

		retirable := false
		if netErr := isRetriableNetworkError(err); netErr {
			retirable = true
		}
		if httpErr, ok := err.(*httpError); ok {
			if isRetriableHTTPError(httpErr.statusCode) {
				retirable = true
			}
		}

		if !retirable {
			return err
		}

		if i == len(delays)-1 {
			return fmt.Errorf("failed after %d attempts: %w", len(delays), err)
		}
	}
	return nil
}
