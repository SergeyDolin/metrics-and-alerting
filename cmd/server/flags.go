package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/SergeyDolin/metrics-and-alerting/internal/config"
)

// Server configuration variables that can be set via command-line flags
// and/or environment variables. Environment variables take precedence
// over command-line flags when both are provided.
var (
	// flagRunAddr specifies the network address and port for the HTTP server to listen on.
	// Format: "host:port" (e.g., "localhost:8080", ":8080")
	// Can be set via flag "-a" or environment variable "ADDRESS"
	flagRunAddr string

	// flagStoreInterval defines how often metrics are saved to disk.
	// If set to 0, metrics are saved synchronously after each update.
	// Can be set via flag "-i" or environment variable "STORE_INTERVAL"
	flagStoreInterval time.Duration

	// flagFileStoragePath specifies the file path where metrics are persisted.
	// Used for restoring metrics after server restart.
	// Can be set via flag "-f" or environment variable "FILE_STORAGE_PATH"
	flagFileStoragePath string

	// flagRestore determines whether to load metrics from the storage file on startup.
	// If true, metrics are restored from flagFileStoragePath when the server starts.
	// Can be set via flag "-r" or environment variable "RESTORE"
	flagRestore bool

	// flagSQL contains the database connection string for persistent storage.
	// If provided, metrics are stored in a database instead of/in addition to files.
	// Format: "postgres://username:password@host:port/database?sslmode=disable"
	// Can be set via flag "-d" or environment variable "DATABASE_DSN"
	flagSQL string

	// flagKey is the secret key used for HMAC-SHA256 signing of requests and responses
	// to ensure data integrity and authenticity between agent and server.
	// Can be set via flag "-k" or environment variable "KEY"
	flagKey string

	// flagAuditFile specifies the file path for writing audit logs.
	// Audit events are written to this file in JSON format, one per line.
	// Can be set via flag "-audit-file" or environment variable "AUDIT_FILE"
	flagAuditFile string

	// flagAuditURL specifies the HTTP endpoint for sending audit logs.
	// Audit events are POSTed to this URL as JSON.
	// Can be set via flag "-audit-url" or environment variable "AUDIT_URL"
	flagAuditURL string

	// flagCryptoKey specifies the path to the private key file for asymmetric encryption.
	// Can be set via flag "-crypto-key" or environment variable "CRYPTO_KEY"
	flagCryptoKey string

	// flagConfigPath specifies the path to the configuration file
	// Can be set via flag "-c" or "-config" or environment variable "CONFIG"
	flagConfigPath string
)

// parseFlags processes command-line arguments and environment variables
// to configure the server. It follows a specific priority order:
// Environment Variable > Command-line Flag > Default Value
//
// The function performs the following steps:
//  1. Defines command-line flags with their default values
//  2. Parses command-line arguments
//  3. Checks for environment variables and overrides flag values if found
//  4. Logs warnings when environment variables are not set (informational)
//
// Supported environment variables:
//   - ADDRESS: Server address (overrides -a)
//   - STORE_INTERVAL: Store interval in seconds (overrides -i)
//   - FILE_STORAGE_PATH: Path to metrics storage file (overrides -f)
//   - RESTORE: Boolean flag to restore metrics on startup (overrides -r)
//   - DATABASE_DSN: Database connection string (overrides -d)
//   - KEY: HMAC secret key (overrides -k)
//   - AUDIT_FILE: Path to audit log file (overrides -audit-file)
//   - AUDIT_URL: URL for audit log endpoint (overrides -audit-url)
//   - CRYPTO_KEY: Path to private key file for asymmetric encryption (overrides -crypto-key)
//
// This function should be called early in the server initialization process,
// typically right after the main() function starts.
func parseFlags() {
	// Define command-line flags with their default values and help text
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")

	// Default store interval is 300 seconds (5 minutes)
	// Use 0 for synchronous saves after each metric update
	flag.DurationVar(&flagStoreInterval, "i", 300*time.Second, "metrics store interval (0 for synchronous save)")

	// Default file path for metrics storage
	flag.StringVar(&flagFileStoragePath, "f", "/tmp/metrics.json", "file path for metrics storage")

	// Default behavior: do not restore metrics from file on startup
	flag.BoolVar(&flagRestore, "r", false, "restore metrics from file on startup")

	// Database connection string (empty by default, meaning no database storage)
	flag.StringVar(&flagSQL, "d", "", "DB address")

	// HMAC key for request/response signing (empty by default, meaning no signing)
	flag.StringVar(&flagKey, "k", "", "HMAC key for request/response signing")

	// Audit log file path (empty by default, meaning no file-based audit logging)
	flag.StringVar(&flagAuditFile, "audit-file", "", "path to audit log file")

	// Audit log URL (empty by default, meaning no HTTP-based audit logging)
	flag.StringVar(&flagAuditURL, "audit-url", "", "URL to send audit logs")

	// Path to private key for asymmetric encryption (empty by default, meaning no encryption)
	flag.StringVar(&flagCryptoKey, "crypto-key", "", "path to private key file for encryption")

	// Path to configuration file (empty by default, meaning no config file is used)
	flag.StringVar(&flagConfigPath, "c", "", "path to config file")
	flag.StringVar(&flagConfigPath, "config", "", "path to config file (alternative flag)")

	// Parse all defined command-line flags
	flag.Parse()

	// Override server address from environment variable if provided
	if address, ok := os.LookupEnv("ADDRESS"); ok {
		flagRunAddr = address
	} else {
		log.Printf("ADDRESS not set\n")
	}

	// Override store interval from environment variable if provided and valid
	if intervalStr, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		if seconds, err := strconv.Atoi(intervalStr); err == nil {
			flagStoreInterval = time.Duration(seconds) * time.Second
		}
	} else {
		log.Printf("STORE_INTERVAL not set\n")
	}

	// Override file storage path from environment variable if provided
	if filePath, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		flagFileStoragePath = filePath
	} else {
		log.Printf("FILE_STORAGE_PATH not set\n")
	}

	// Override restore flag from environment variable if provided
	// Environment variable expects "true" or "false" as string values
	if restoreStr, ok := os.LookupEnv("RESTORE"); ok {
		flagRestore = restoreStr == "true"
	} else {
		log.Printf("RESTORE not set\n")
	}

	// Override database DSN from environment variable if provided
	if dbName, ok := os.LookupEnv("DATABASE_DSN"); ok {
		flagSQL = dbName
	} else {
		log.Printf("DATABASE_DSN not set\n")
	}

	// Override HMAC key from environment variable if provided
	if key, ok := os.LookupEnv("KEY"); ok {
		flagKey = key
	} else {
		log.Printf("KEY not set\n")
	}

	// Override audit file path from environment variable if provided
	if auditFile, ok := os.LookupEnv("AUDIT_FILE"); ok {
		flagAuditFile = auditFile
	} else {
		log.Printf("AUDIT_FILE not set")
	}

	// Override audit URL from environment variable if provided
	if auditURL, ok := os.LookupEnv("AUDIT_URL"); ok {
		flagAuditURL = auditURL
	} else {
		log.Printf("AUDIT_URL not set")
	}

	// Override crypto key path from environment variable if provided
	if cryptoKey, ok := os.LookupEnv("CRYPTO_KEY"); ok {
		flagCryptoKey = cryptoKey
	} else {
		log.Printf("CRYPTO_KEY not set")
	}

	// Load configuration from file if provided
	configPath := flagConfigPath
	if envConfigPath := os.Getenv("CONFIG"); envConfigPath != "" {
		configPath = envConfigPath
	}

	if configPath != "" {
		serverConfig, err := config.LoadServerConfig(configPath)
		if err == nil {
			// Apply config values only if not already set by flags or environment variables
			if flagRunAddr == "localhost:8080" {
				flagRunAddr = serverConfig.Address
			}
			if flagStoreInterval == 300*time.Second {
				storeInterval, err := time.ParseDuration(serverConfig.StoreInterval)
				if err == nil {
					flagStoreInterval = storeInterval
				}
			}
			if flagFileStoragePath == "/tmp/metrics.json" {
				flagFileStoragePath = serverConfig.StoreFile
			}
			if !flagRestore {
				flagRestore = serverConfig.Restore
			}
			if flagSQL == "" {
				flagSQL = serverConfig.DB.DSN
			}
			if flagCryptoKey == "" {
				flagCryptoKey = serverConfig.CryptoKey
			}
		} else {
			log.Printf("Failed to load config file: %v", err)
		}
	}
}
