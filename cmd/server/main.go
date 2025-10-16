package main

import (
	"net/http"

	"github.com/go-chi/chi"

	"go.uber.org/zap"
)

// main — точка входа приложения.
// Инициализирует роутер chi, создаёт хранилище метрик и настраивает маршруты:
// - GET / — список всех метрик
// - POST /update/{type}/{name}/{value} — обновление метрики
// - GET /value/{type}/{name} — получение значения метрики
// Запускает HTTP-сервер на порту 8080.
// Также задаёт глобальные обработчики для MethodNotAllowed и NotFound.
func main() {
	parseFlags()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("cannot initialize zap")
	}
	defer logger.Sync()

	sugar := logger.Sugar()

	router := chi.NewRouter()
	ms := createMetricStorage()

	// router.Use(recoverMiddleware(sugar))
	router.Use(logMiddleware(sugar))

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Route("/", func(r chi.Router) {
		r.Get("/", indexHandler(ms))

		r.Post("/update", updateJSONHandler(ms))
		r.Post("/value", valueJSONHandler(ms))

		r.Post("/update/{type}/{name}/{value}", postHandler(ms))
		r.Get("/value/{type}/{name}", getHandler(ms))

	})
	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))
}
