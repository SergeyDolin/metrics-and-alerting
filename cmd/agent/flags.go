package main

import (
	"flag"
	"log"
	"os"
	"strconv"
)

// Command-line flags and environment variables configuration for the metrics agent.
// The agent supports configuration through both command-line flags and environment variables,
// with environment variables taking precedence when both are provided.

var (
	// sAddr specifies the server address and port to send metrics to.
	// Can be set via command-line flag "-a" or environment variable "ADDRESS".
	// Default value: "localhost:8080"
	sAddr = flag.String("a", "localhost:8080", "address and port to run server")

	// pInterval defines how often metrics are polled from the system (in seconds).
	// Can be set via command-line flag "-p" or environment variable "POLL_INTERVAL".
	// Default value: 2 seconds
	pInterval = flag.Int("p", 2, "pollInterval set")

	// rInterval defines how often collected metrics are reported to the server (in seconds).
	// Can be set via command-line flag "-r" or environment variable "REPORT_INTERVAL".
	// Default value: 10 seconds
	rInterval = flag.Int("r", 10, "reportInterval set")

	// key is the secret key used for HMAC-SHA256 signing of requests to ensure data integrity.
	// Can be set via command-line flag "-k" or environment variable "KEY".
	// Default value: empty string (no signing)
	key = flag.String("k", "", "key set")

	// rateLimit limits the number of concurrent outgoing requests to the server.
	// Can be set via command-line flag "-l" or environment variable "RATE_LIMIT".
	// Default value: 1 (single concurrent request)
	rateLimit = flag.Int("l", 1, "rate limit set")

	// cryptoKey specifies the path to the public key file for asymmetric encryption.
	// Can be set via command-line flag "-crypto-key" or environment variable "CRYPTO_KEY".
	// Default value: empty string (no encryption)
	cryptoKey = flag.String("crypto-key", "", "path to public key file for encryption")
)

// parseArgs processes command-line arguments and environment variables to configure the agent.
// It first parses any command-line flags provided, then checks for environment variables.
// Environment variables take precedence over command-line flags if both are set.
//
// The following environment variables are supported:
//   - ADDRESS: Overrides the server address (overrides -a flag)
//   - POLL_INTERVAL: Overrides the polling interval (overrides -p flag)
//   - REPORT_INTERVAL: Overrides the reporting interval (overrides -r flag)
//   - KEY: Overrides the HMAC secret key (overrides -k flag)
//   - RATE_LIMIT: Overrides the rate limit (overrides -l flag)
//   - CRYPTO_KEY: Overrides the path to the public key file (overrides -crypto-key flag)
//
// The function logs warnings when:
//   - Environment variables are not set (informational)
//   - Integer environment variables (POLL_INTERVAL, REPORT_INTERVAL, RATE_LIMIT)
//     contain invalid values that cannot be parsed
//
// This function should be called early in the program initialization,
// typically right after the main() function starts.
func parseArgs() {
	// Parse command-line flags with their default values
	flag.Parse()

	// Override server address from environment variable if provided
	if addressOs, ok := os.LookupEnv("ADDRESS"); ok {
		*sAddr = addressOs
	} else {
		log.Printf("%s not set\n", addressOs)
	}

	// Override poll interval from environment variable if provided and valid
	if pollIntervalOs, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		if pInter, err := strconv.Atoi(pollIntervalOs); err == nil {
			*pInterval = pInter
		} else {
			log.Printf("Invalid POLL_INTERVAL value '%s': %v", pollIntervalOs, err)
		}
	} else {
		log.Printf("%s not set\n", pollIntervalOs)
	}

	// Override report interval from environment variable if provided and valid
	if reportIntervalOs, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		if rInter, err := strconv.Atoi(reportIntervalOs); err == nil {
			*rInterval = rInter
		} else {
			log.Printf("Invalid REPORT_INTERVAL value '%s': %v", reportIntervalOs, err)
		}
	} else {
		log.Printf("%s not set\n", reportIntervalOs)
	}

	// Override HMAC secret key from environment variable if provided
	if keyOs, ok := os.LookupEnv("KEY"); ok {
		*key = keyOs
	} else {
		log.Printf("%s not set\n", keyOs)
	}

	// Override rate limit from environment variable if provided and valid
	if rateLim, ok := os.LookupEnv("RATE_LIMIT"); ok {
		if rLimit, err := strconv.Atoi(rateLim); err == nil {
			*rateLimit = rLimit
		} else {
			log.Printf("Invalid RATE_LIMIT value '%s': %v", rateLim, err)
		}
	} else {
		log.Printf("%s not set\n", rateLim)
	}

	// Override crypto key path from environment variable if provided
	if cryptoKeyOs, ok := os.LookupEnv("CRYPTO_KEY"); ok {
		*cryptoKey = cryptoKeyOs
	} else {
		log.Printf("%s not set\n", cryptoKeyOs)
	}
}
