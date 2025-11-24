package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"
)

var (
	flagRunAddr         string
	flagStoreInterval   time.Duration
	flagFileStoragePath string
	flagRestore         bool
	flagSQL             string
	flagKey             string
)

// parseFlags обрабатывает аргументы командной строки и переменные окружения.
// Приоритет: переменная окружения > флаг > значение по умолчанию.
func parseFlags() {

	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")

	// По умолчанию 300 секунд (5 минут)
	flag.DurationVar(&flagStoreInterval, "i", 300*time.Second, "metrics store interval (0 for synchronous save)")

	// Имя файла по умолчанию — выберем /tmp/metrics.json (можно изменить)
	flag.StringVar(&flagFileStoragePath, "f", "/tmp/metrics.json", "file path for metrics storage")

	flag.BoolVar(&flagRestore, "r", false, "restore metrics from file on startup")

	flag.StringVar(&flagSQL, "d", "", "DB address")

	flag.StringVar(&flagKey, "k", "", "HMAC key for request/response signing")

	flag.Parse()

	if address, ok := os.LookupEnv("ADDRESS"); ok {
		flagRunAddr = address
	} else {
		log.Printf("ADDRESS not set\n")
	}

	if intervalStr, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		if seconds, err := strconv.Atoi(intervalStr); err == nil {
			flagStoreInterval = time.Duration(seconds) * time.Second
		}
	} else {
		log.Printf("STORE_INTERVAL not set\n")
	}

	if filePath, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		flagFileStoragePath = filePath
	} else {
		log.Printf("FILE_STORAGE_PATH not set\n")
	}

	if restoreStr, ok := os.LookupEnv("RESTORE"); ok {
		flagRestore = restoreStr == "true"
	} else {
		log.Printf("RESTORE not set\n")
	}

	if dbName, ok := os.LookupEnv("DATABASE_DSN"); ok {
		flagSQL = dbName
	} else {
		log.Printf("DATABASE_DSN not set\n")
	}

	if key, ok := os.LookupEnv("KEY"); ok {
		flagKey = key
	} else {
		log.Printf("KEY not set\n")
	}
}
