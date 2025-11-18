package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/SergeyDolin/metrics-and-alerting/internal/metrics"
	"github.com/SergeyDolin/metrics-and-alerting/internal/pgerrors"
)

type DBStorage struct {
	conn  *pgx.Conn
	cache *MemStorage
	mu    sync.RWMutex
}

func NewDBStorage(dsn string) (*DBStorage, error) {
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to DB: %w", err)
	}

	s := &DBStorage{
		conn:  conn,
		cache: NewMemStorage(),
	}

	if err := s.initSchema(); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("init schema: %w", err)
	}

	if err := s.loadFromDB(); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("load from DB: %w", err)
	}

	return s, nil
}

func (s *DBStorage) initSchema() error {
	_, err := s.conn.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS gauge (
			name VARCHAR(255) PRIMARY KEY,
			value DOUBLE PRECISION NOT NULL
		);
		CREATE TABLE IF NOT EXISTS counter (
			name VARCHAR(255) PRIMARY KEY,
			value BIGINT NOT NULL
		);
	`)
	return err
}

func (s *DBStorage) loadFromDB() error {
	rows, err := s.conn.Query(context.Background(), `SELECT name, value FROM gauge`)
	if err != nil {
		return fmt.Errorf("query gauge: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value float64
		if err := rows.Scan(&name, &value); err != nil {
			return fmt.Errorf("scan gauge: %w", err)
		}
		s.cache.gauge[name] = value
	}

	rows, err = s.conn.Query(context.Background(), `SELECT name, value FROM counter`)
	if err != nil {
		return fmt.Errorf("query counter: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value int64
		if err := rows.Scan(&name, &value); err != nil {
			return fmt.Errorf("scan counter: %w", err)
		}
		s.cache.counter[name] = value
	}
	return nil
}

func (s *DBStorage) execWithRetry(ctx context.Context, query string, args ...interface{}) error {
	classifier := pgerrors.NewPostgresErrorClassifier()
	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

	var err error
	for attempt := 0; attempt <= len(delays); attempt++ {
		_, err = s.conn.Exec(ctx, query, args...)
		if err == nil {
			return nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && classifier.Classify(err) == pgerrors.Retriable {
			if attempt < len(delays) {
				time.Sleep(delays[attempt])
				continue
			}
		}
		return err
	}
	return err
}

func (s *DBStorage) UpdateGauge(name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO gauge (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`
	if err := s.execWithRetry(context.Background(), query, name, value); err != nil {
		return fmt.Errorf("save gauge %s: %w", name, err)
	}
	s.cache.gauge[name] = value
	return nil
}

func (s *DBStorage) UpdateCounter(name string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO counter (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = counter.value + $2`
	if err := s.execWithRetry(context.Background(), query, name, delta); err != nil {
		return fmt.Errorf("save counter %s: %w", name, err)
	}

	s.cache.counter[name] += delta
	return nil
}

func (s *DBStorage) SaveCounterValue(name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.conn.Exec(
		context.Background(),
		`INSERT INTO counter (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`,
		name, value,
	)
	if err != nil {
		return fmt.Errorf("save counter value %s: %w", name, err)
	}
	s.cache.counter[name] = value
	return nil
}

func (s *DBStorage) SaveAll() error {
	s.mu.RLock()
	gauges := make(map[string]float64)
	counters := make(map[string]int64)
	for k, v := range s.cache.gauge {
		gauges[k] = v
	}
	for k, v := range s.cache.counter {
		counters[k] = v
	}
	s.mu.RUnlock()

	if len(gauges) == 0 && len(counters) == 0 {
		return nil
	}

	batch := &pgx.Batch{}

	for name, value := range gauges {
		batch.Queue(
			`INSERT INTO gauge (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`,
			name, value,
		)
	}

	for name, value := range counters {
		batch.Queue(
			`INSERT INTO counter (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`,
			name, value,
		)
	}

	results := s.conn.SendBatch(context.Background(), batch)
	defer results.Close()

	var firstErr error
	total := len(gauges) + len(counters)
	for i := 0; i < total; i++ {
		if _, err := results.Exec(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func (s *DBStorage) Ping(ctx context.Context) error {
	return s.conn.PgConn().Ping(ctx)
}

func (s *DBStorage) GetGauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetGauge(name)
}

func (s *DBStorage) GetCounter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetCounter(name)
}

func (s *DBStorage) GetAll() ([]metrics.Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetAll()
}

func (s *DBStorage) Close() error {
	return s.conn.Close(context.Background())
}
