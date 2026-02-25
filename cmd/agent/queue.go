package main

import (
	"log"
	"sync"
)

type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
}

type MetricQueue struct {
	queue chan Metrics
	pool  sync.Pool
}

func NewMetricQueue(size int) *MetricQueue {
	return &MetricQueue{
		queue: make(chan Metrics, 100),
		pool: sync.Pool{
			New: func() interface{} {
				return &Metrics{}
			},
		},
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
	return len(mq.queue) == 0
}
