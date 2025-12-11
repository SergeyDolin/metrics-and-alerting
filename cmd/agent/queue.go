package main

import "log"

type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
}

type MetricQueue struct {
	queue chan Metrics
}

func NewMetricQueue(size int) *MetricQueue {
	return &MetricQueue{
		queue: make(chan Metrics, size),
	}
}

func (mq *MetricQueue) Push(metric Metrics) {
	select {
	case mq.queue <- metric:
	default:
		log.Printf("Queue full, dropping metric %s\n", metric.ID)
	}
}

func (mq *MetricQueue) Pop() Metrics {
	return <-mq.queue
}

func (mq *MetricQueue) IsEmpty() bool {
	select {
	case <-mq.queue:
		return false
	default:
		return true
	}
}
