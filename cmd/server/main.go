package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/go-chi/chi"
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
}

// main — точка входа приложения.
// Инициализирует роутер chi, создаёт хранилище метрик и настраивает маршруты:
// - GET / — список всех метрик
// - POST /update/{type}/{name}/{value} — обновление метрики
// - GET /value/{type}/{name} — получение значения метрики
// Запускает HTTP-сервер на порту 8080.
// Также задаёт глобальные обработчики для MethodNotAllowed и NotFound.
func main() {
	parseFlags()

	router := chi.NewRouter()
	ms := createMetricStorage()

	router.Use(recoverMiddleware)
	router.Use(logMiddleware)

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Route("/", func(r chi.Router) {
		r.Get("/", indexHandler(ms))
		r.Route("/update", func(r chi.Router) {
			r.Post("/{type}/{name}/{value}", postHandler(ms))
		})
		r.Route("/value", func(r chi.Router) {
			r.Get("/{type}/{name}", getHandler(ms))
		})
	})
	log.Printf("Running server on %s", flagRunAddr)
	log.Fatal(http.ListenAndServe(flagRunAddr, router))
}
