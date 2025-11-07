package main

import (
	"context"
	"database/sql"
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
		panic("cannot initialize zap")
	}
	defer logger.Sync()
	sugar := logger.Sugar()

	router := chi.NewRouter()
	var ms *MetricStorage
	var saveSync func()

	if flagSQL != "" {
		sugar.Infof("Initializing PostgreSQL storage with DSN: %s", flagSQL)

		db, err := sql.Open("pgx", flagSQL)
		if err != nil {
			sugar.Fatalf("Failed to open DB connection: %v", err)
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err != nil {
			sugar.Fatalf("Failed to ping DB: %v", err)
		}

		if err := storage.RunMigrations(db); err != nil {
			sugar.Fatalf("Migrations failed: %v", err)
		}

		ms = &MetricStorage{
			gauge:   make(map[string]float64),
			counter: make(map[string]int64),
			db:      db,
		}

		if err := ms.loadFromDB(); err != nil {
			sugar.Warnf("Failed to load metrics from DB: %v", err)
		}

		saveSync = func() {
			ms.saveToDB()
		}

	} else {
		ms = &MetricStorage{
			gauge:   make(map[string]float64),
			counter: make(map[string]int64),
		}

		if flagFileStoragePath != "" && flagRestore {
			if err := ms.LoadFromFile(flagFileStoragePath); err != nil {
				sugar.Warnf("Failed to restore metrics from file: %v", err)
			}
		}

		// Фоновое сохранение
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

		// Обёртка сохранения для файла/памяти
		saveSync = func() {
			if flagStoreInterval == 0 && flagFileStoragePath != "" {
				if err := ms.SaveToFile(flagFileStoragePath); err != nil {
					sugar.Errorf("Sync save failed: %v", err)
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

	router.Get("/", indexHandler(ms))
	router.Post("/update", updateJSONHandler(ms, saveSync))
	router.Get("/ping", pingSQLHandler(flagSQL))
	router.Post("/value", valueJSONHandler(ms))
	router.Post("/update/{type}/{name}/{value}", postHandler(ms, saveSync))
	router.Get("/value/{type}/{name}", getHandler(ms))

	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))
}
