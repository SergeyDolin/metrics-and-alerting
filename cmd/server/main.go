package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

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

// MetricStorage — структура для хранения метрик двух типов: gauge (произвольное значение) и counter (счётчик, только инкремент)
type MetricStorage struct {
	gauge   map[string]float64 // Хранит метрики типа gauge (например использование памяти)
	counter map[string]int64   // Хранит метрики типа counter (например количество запросов или ошибок)
}

// createMetricStorage — создаёт и инициализирует новый экземпляр хранилища метрик.
// Возвращает указатель на MetricStorage с инициализированными пустыми мапами для gauge и counter.
func createMetricStorage() *MetricStorage {
	return &MetricStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
}

// updateGauge — обновляет или устанавливает значение метрики типа gauge по имени.
// Перезаписывает текущее значение, если оно существует.
func (ms *MetricStorage) updateGauge(name string, value float64) {
	ms.gauge[name] = value
}

// updateCounter — обновляет значение метрики типа counter по имени.
// Если метрика ещё не существует — инициализирует её нулём, затем прибавляет переданное значение.
// Counter предназначен для накопления, а не перезаписи.
func (ms *MetricStorage) updateCounter(name string, value int64) {
	if _, ok := ms.counter[name]; !ok {
		ms.counter[name] = 0
	}
	ms.counter[name] += value
}

// indexHandler — возвращает HTTP-обработчик, который выводит все метрики (gauge и counter) в виде строки.
// Формат: "metric1=value1, metric2=value2, ..."
// Поддерживает только GET-запросы. При других методах возвращает ошибку 405.
func indexHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}
		list := make([]string, 0)
		for name, value := range ms.gauge {
			list = append(list, fmt.Sprintf("%s=%v", name, value))
		}
		for name, value := range ms.counter {
			list = append(list, fmt.Sprintf("%s=%v", name, value))
		}
		io.WriteString(res, strings.Join(list, ", "))
	}
}

// getHandler — возвращает HTTP-обработчик для получения значения конкретной метрики по типу и имени.
// URL: /value/{type}/{name}
// Поддерживает только GET. Возвращает значение метрики или ошибку 404, если метрика не найдена.
// Типы: "gauge" или "counter". Регистр не важен.
func getHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Only GET request allowed!", http.StatusMethodNotAllowed)
			return
		}

		metricType := strings.ToLower(chi.URLParam(req, "type"))
		metricName := chi.URLParam(req, "name")

		switch metricType {
		case "gauge":
			if value, exists := ms.gauge[metricName]; exists {
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		case "counter":
			if value, exists := ms.counter[metricName]; exists {
				io.WriteString(res, fmt.Sprintf("%v", value))
				return
			}
			http.Error(res, "Unknown metric name", http.StatusNotFound)
			return

		default:
			http.Error(res, "Unknown metric type", http.StatusNotFound)
			return
		}
	}
}

// postHandler — возвращает HTTP-обработчик для обновления метрик через POST-запрос.
// URL: /update/{type}/{name}/{value}
// Поддерживает только POST. Валидирует тип значения в зависимости от типа метрики:
// - gauge: требует float64
// - counter: требует int64
// При успехе возвращает 200 OK, при ошибках — соответствующие HTTP-ошибки.
func postHandler(ms *MetricStorage) func(http.ResponseWriter, *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Only POST request allowed!", http.StatusMethodNotAllowed)
			return
		}

		typeOfMetric := strings.ToLower(chi.URLParam(req, "type"))
		nameOfMetric := chi.URLParam(req, "name")
		valueOfMetric := chi.URLParam(req, "value")

		switch typeOfMetric {
		case "gauge":
			value, err := strconv.ParseFloat(valueOfMetric, 64)
			if err != nil {
				http.Error(res, "Only Float type for Gauge allowed!", http.StatusBadRequest)
				return
			}
			ms.updateGauge(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)

		case "counter":
			value, err := strconv.ParseInt(valueOfMetric, 10, 64)
			if err != nil {
				http.Error(res, "Only Int type for Counter allowed!", http.StatusBadRequest)
				return
			}
			ms.updateCounter(nameOfMetric, value)
			res.WriteHeader(http.StatusOK)

		default:
			http.Error(res, "Unknown metric type", http.StatusBadRequest)
			return
		}
	}
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
	fmt.Println("Running server on", flagRunAddr)
	log.Fatal(http.ListenAndServe(flagRunAddr, router))
}
