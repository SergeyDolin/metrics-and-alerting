package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi"

	"go.uber.org/zap"

	"github.com/go-chi/chi/middleware"
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

	// Загрузка при старте
	if flagRestore && flagFileStoragePath != "" {
		if err := ms.LoadFromFile(flagFileStoragePath); err != nil {
			sugar.Warnf("Failed to restore metrics: %v", err)
		}
	}

	// Фоновое сохранение, если интервал > 0
	if flagStoreInterval > 0 && flagFileStoragePath != "" {
		go func() {
			ticker := time.NewTicker(flagStoreInterval)
			defer ticker.Stop()
			for range ticker.C {
				if err := ms.SaveToFile(flagFileStoragePath); err != nil {
					sugar.Errorf("Periodic save failed: %v", err)
				}
			}
		}()
	}

	// Обёртка для синхронного сохранения
	saveSync := func() {
		if flagStoreInterval == 0 && flagFileStoragePath != "" {
			if err := ms.SaveToFile(flagFileStoragePath); err != nil {
				sugar.Errorf("Sync save failed: %v", err)
			}
		}
	}

	// router.Use(recoverMiddleware(sugar))
	router.Use(middleware.StripSlashes)
	router.Use(gzipMiddleware)
	router.Use(logMiddleware(sugar))

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Get("/", indexHandler(ms))
	router.Post("/update", updateJSONHandler(ms, saveSync))
	router.Get("/ping", pingSQLHandler(flagSql))
	router.Post("/value", valueJSONHandler(ms))
	router.Post("/update/{type}/{name}/{value}", postHandler(ms, saveSync))
	router.Get("/value/{type}/{name}", getHandler(ms))

	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))
}
