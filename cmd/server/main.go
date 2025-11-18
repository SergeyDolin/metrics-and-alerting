package main

import (
	"net/http"
	"time"

	"github.com/SergeyDolin/metrics-and-alerting/internal/storage"
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
		logger.Fatal("cannot initialize zap")
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	router := chi.NewRouter()
	var store storage.Storage
	var saveSync func()

	if flagSQL != "" {
		sugar.Infof("Initializing PostgreSQL storage with DSN: %s", flagSQL)

		dbStorage, err := storage.NewDBStorage(flagSQL)
		if err != nil {
			sugar.Fatalf("Failed to open DB connection: %v", err)
		}
		defer func() {
			if err := dbStorage.SaveAll(); err != nil {
				sugar.Errorf("Failed to save metrics on exit: %v", err)
			}
			dbStorage.Close()
		}()
		store = dbStorage
	} else {
		if flagFileStoragePath != "" && flagRestore {
			fileStorage, err := storage.NewFileStorage(flagFileStoragePath)
			if err != nil {
				sugar.Warnf("Failed to restore metrics from file: %v", err)
			} else {
				store = fileStorage
			}
		}
		if store == nil {
			store = storage.NewMemStorage()
		}
		// Фоновое сохранение
		if flagStoreInterval > 0 && flagFileStoragePath != "" {
			if fs, ok := store.(*storage.FileStorage); ok {
				go func() {
					ticker := time.NewTicker(flagStoreInterval)
					defer ticker.Stop()
					for range ticker.C {
						if err := fs.Save(); err != nil {
							sugar.Errorf("Periodic save failed: %v", err)
						}
					}
				}()
			}
		}

		// Обёртка сохранения для файла/памяти
		saveSync = func() {
			if flagStoreInterval == 0 && flagFileStoragePath != "" {
				if fs, ok := store.(*storage.FileStorage); ok {
					if err := fs.Save(); err != nil {
						sugar.Errorf("Sync save failed: %v", err)
					}
				}
			}
		}
	}

	router.Use(middleware.StripSlashes)
	router.Use(gzipMiddleware)
	router.Use(logMiddleware(sugar))

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Invalid path format", http.StatusNotFound)
	})

	router.Get("/", indexHandler(store))
	router.Post("/update", updateJSONHandler(store, saveSync))
	router.Post("/updates", updatesBatchHandler(store, saveSync))
	router.Get("/ping", pingSQLHandler(store))
	router.Post("/value", valueJSONHandler(store))
	router.Post("/update/{type}/{name}/{value}", postHandler(store, saveSync))
	router.Get("/value/{type}/{name}", getHandler(store))

	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))
}
