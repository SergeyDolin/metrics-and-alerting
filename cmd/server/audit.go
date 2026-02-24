package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type AuditEvent struct {
	Timestamp int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

type Observer interface {
	Notify(AuditEvent) error
	Close() error
}

type Publisher struct {
	observers []Observer
	mutex     sync.RWMutex
}

func NewPublisher(observers []Observer) *Publisher {
	return &Publisher{
		observers: observers,
	}
}

func (p *Publisher) Register(observer Observer) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.observers = append(p.observers, observer)
}

func (p *Publisher) Notify(event AuditEvent) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	for _, observer := range p.observers {
		if err := observer.Notify(event); err != nil {
			fmt.Fprintf(os.Stderr, "Error notifying observer: %v\n", err)
		}
	}
}

func (p *Publisher) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, observer := range p.observers {
		observer.Close()
	}
}

type FileWriterObserver struct {
	filePath string
	mutex    sync.Mutex
}

func NewFileWriterObserver(filePath string) *FileWriterObserver {
	return &FileWriterObserver{
		filePath: filePath,
	}
}

func (fw *FileWriterObserver) Notify(event AuditEvent) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	file, err := os.OpenFile(fw.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to audit file: %w", err)
	}

	_, err = file.WriteString("\n")
	if err != nil {
		return fmt.Errorf("failed to write newline to audit file: %w", err)
	}

	return nil
}

func (fw *FileWriterObserver) Close() error {
	return nil
}

type HTTPSenderObserver struct {
	url    string
	client *http.Client
}

func NewHTTPSenderObserver(url string) *HTTPSenderObserver {
	return &HTTPSenderObserver{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (hs *HTTPSenderObserver) Notify(event AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	resp, err := hs.client.Post(hs.url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to send audit event to %s: %w", hs.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-success status code %d from %s", resp.StatusCode, hs.url)
	}

	return nil
}

func (hs *HTTPSenderObserver) Close() error {
	return nil
}
