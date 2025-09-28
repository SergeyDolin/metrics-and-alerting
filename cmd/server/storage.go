package main

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
