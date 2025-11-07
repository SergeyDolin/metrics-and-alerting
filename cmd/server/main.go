package main

import (
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
	var closeDB func()

	// Загрузка при старте
	if flagSQL != "" {
		db, err := sql.Open("pgx", flagSQL)
		if err != nil {
			sugar.Fatalf("Failed to open DB: %v", err)
		}
		if err := db.Ping(); err != nil {
			db.Close()
			sugar.Fatalf("Failed to ping DB: %v", err)
		}

		if err := storage.RunMigrations(db); err != nil {
			db.Close()
			sugar.Fatalf("Migrations failed: %v", err)
		}

		ms = &MetricStorage{
			gauge:   make(map[string]float64),
			counter: make(map[string]int64),
			db:      db,
		}

		if err := ms.loadFromDB(); err != nil {
			sugar.Warnf("Failed to load metrics: %v", err)
		}

		closeDB = func() {
			if ms.db != nil {
				ms.db.Close()
			}
		}
	} else if flagFileStoragePath != "" {
		ms, err = createMetricStorage("")
		if err != nil {
			sugar.Warnf("Failed DB: %v", err)
		}
		if flagRestore {
			if err := ms.LoadFromFile(flagFileStoragePath); err != nil {
				sugar.Warnf("Failed to restore metrics from file: %v", err)
			}
		}
		closeDB = func() {
			// Синхронное сохранение при завершении
			if flagStoreInterval == 0 && flagFileStoragePath != "" {
				if err := ms.SaveToFile(flagFileStoragePath); err != nil {
					sugar.Errorf("Final save failed: %v", err)
				}
			}
		}
	} else {
		ms, err = createMetricStorage("")
		if err != nil {
			sugar.Warnf("Failed DB: %v", err)
		}
		closeDB = func() {}
	}

	// Фоновое сохранение, если интервал > 0 и нет базы данных
	if flagStoreInterval > 0 && flagFileStoragePath != "" && flagSQL == "" {
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
		if flagSQL != "" {
			ms.saveToDB()
		} else if flagStoreInterval == 0 && flagFileStoragePath != "" {
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
	router.Get("/ping", pingSQLHandler(flagSQL))
	router.Post("/value", valueJSONHandler(ms))
	router.Post("/update/{type}/{name}/{value}", postHandler(ms, saveSync))
	router.Get("/value/{type}/{name}", getHandler(ms))

	sugar.Infof("Running server on %s", flagRunAddr)
	sugar.Fatal(http.ListenAndServe(flagRunAddr, router))

	closeDB()
}
