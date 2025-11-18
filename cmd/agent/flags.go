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
	key       = flag.String("k", "", "key set")
)

func parseArgs() {
	// Парсим флаги по умолчанию
	flag.Parse()

	// Переопределяем значения из переменных окружения, если они заданы
	if addressOs, ok := os.LookupEnv("ADDRESS"); ok {
		*sAddr = addressOs
	} else {
		log.Printf("%s not set\n", addressOs)
	}

	if pollIntervalOs, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		if pInter, err := strconv.Atoi(pollIntervalOs); err == nil {
			*pInterval = pInter
		} else {
			log.Printf("Invalid POLL_INTERVAL value '%s': %v", pollIntervalOs, err)
		}
	} else {
		log.Printf("%s not set\n", pollIntervalOs)
	}

	if reportIntervalOs, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		if rInter, err := strconv.Atoi(reportIntervalOs); err == nil {
			*rInterval = rInter
		} else {
			log.Printf("Invalid REPORT_INTERVAL value '%s': %v", reportIntervalOs, err)
		}
	} else {
		log.Printf("%s not set\n", reportIntervalOs)
	}

	if keyOs, ok := os.LookupEnv("KEY"); ok {
		*key = keyOs
	} else {
		log.Printf("%s not set\n", keyOs)
	}
}
