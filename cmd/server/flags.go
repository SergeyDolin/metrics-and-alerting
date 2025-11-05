package main

import (
	"flag"
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
)

// parseFlags обрабатывает аргументы командной строки и переменные окружения.
// Приоритет: переменная окружения > флаг > значение по умолчанию.
func parseFlags() {
	// --- ADDRESS ---
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")

	// --- STORE_INTERVAL ---
	// По умолчанию 300 секунд (5 минут)
	flag.DurationVar(&flagStoreInterval, "i", 300*time.Second, "metrics store interval (0 for synchronous save)")

	// --- FILE_STORAGE_PATH ---
	// Имя файла по умолчанию — выберем /tmp/metrics.json (можно изменить)
	flag.StringVar(&flagFileStoragePath, "f", "/tmp/metrics.json", "file path for metrics storage")

	// --- RESTORE ---
	flag.BoolVar(&flagRestore, "r", false, "restore metrics from file on startup")

	// --- SQL ---
	flag.StringVar(&flagSQL, "d", "video", "DB address")

	// Парсим флаги
	flag.Parse()

	// Переопределяем значения из переменных окружения, если они заданы

	if address := os.Getenv("ADDRESS"); address != "" {
		flagRunAddr = address
	}

	if intervalStr := os.Getenv("STORE_INTERVAL"); intervalStr != "" {
		if seconds, err := strconv.Atoi(intervalStr); err == nil {
			flagStoreInterval = time.Duration(seconds) * time.Second
		}
		// Если не удалось распарсить — оставляем значение из флага (уже задано)
	}

	if filePath := os.Getenv("FILE_STORAGE_PATH"); filePath != "" {
		flagFileStoragePath = filePath
	}

	if restoreStr := os.Getenv("RESTORE"); restoreStr != "" {
		// Сравниваем case-insensitive, но по ТЗ — true/false, так что достаточно == "true"
		flagRestore = restoreStr == "true"
	}

	if dbName := os.Getenv("DATABASE_DSN"); dbName != "" {
		flagSQL = dbName
	}
}
