package main

import (
	"flag"
	"os"
)

// переменная flagRunAddr содержит адрес и порт для запуска сервера
var flagRunAddr string

// parseFlags обрабатывает аргументы командной строки
// и сохраняет их значения в соответствующих переменных
func parseFlags() {
	// регистрируем переменную flagRunAddr
	// как аргумент -a со значением localhost:8080 по умолчанию
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")
	// парсим переданные серверу аргументы в зарегистрированные переменные
	flag.Parse()

	// Переопределяем значения из переменных окружения, если они заданы
	if addressOs := os.Getenv("ADDRESS"); addressOs != "" {
		flagRunAddr = addressOs
	}
}
