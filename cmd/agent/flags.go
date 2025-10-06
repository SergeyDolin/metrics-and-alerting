package main

import (
	"flag"
	"log"
	"os"
	"strconv"
)

// -a=localhost:8080 -r=10 (reportInterval) -p=2 (pollInterval)
var (
	sAddr     = flag.String("a", "localhost:8080", "address and port to run server")
	pInterval = flag.Int("r", 10, "reportInterval set")
	rInterval = flag.Int("p", 2, "pollInterval set")
)

func parseArgs() {
	// Парсим флаги по умолчанию
	flag.Parse()

	// Переопределяем значения из переменных окружения, если они заданы
	if addressOs := os.Getenv("ADDRESS"); addressOs != "" {
		*sAddr = addressOs
	}

	if pollIntervalOs := os.Getenv("POLL_INTERVAL"); pollIntervalOs != "" {
		if pInter, err := strconv.Atoi(pollIntervalOs); err == nil {
			*pInterval = pInter
		} else {
			log.Printf("Invalid POLL_INTERVAL value '%s': %v", pollIntervalOs, err)
		}
	}

	if reportIntervalOs := os.Getenv("REPORT_INTERVAL"); reportIntervalOs != "" {
		if rInter, err := strconv.Atoi(reportIntervalOs); err == nil {
			*rInterval = rInter
		} else {
			log.Printf("Invalid REPORT_INTERVAL value '%s': %v", reportIntervalOs, err)
		}
	}
}
