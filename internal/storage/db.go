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

// DBStorage implements the Storage interface using PostgreSQL as the persistent backend.
// It maintains an in-memory cache (MemStorage) for fast reads and synchronizes writes
// to the database with retry logic for transient failures.
//
// The storage uses two tables:
//   - gauge: Stores floating-point metrics (name VARCHAR PRIMARY KEY, value DOUBLE PRECISION)
//   - counter: Stores integer counter metrics (name VARCHAR PRIMARY KEY, value BIGINT)
//
// All write operations are protected by a mutex to ensure consistency between
// the database and in-memory cache.
type DBStorage struct {
	conn  *pgx.Conn    // PostgreSQL connection
	cache *MemStorage  // In-memory cache for fast reads
	mu    sync.RWMutex // Mutex for thread-safe cache operations
}

// NewDBStorage creates and initializes a new DBStorage instance.
// It performs the following steps:
//  1. Establishes a connection to the PostgreSQL database using the provided DSN
//  2. Initializes the database schema (creates tables if they don't exist)
//  3. Loads existing metrics from the database into the in-memory cache
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/metrics")
//
// Returns:
//   - *DBStorage: Initialized database storage
//   - error: Any error during connection, schema initialization, or data loading
func NewDBStorage(dsn string) (*DBStorage, error) {
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to DB: %w", err)
	}

	s := &DBStorage{
		conn:  conn,
		cache: NewMemStorage(),
	}

	// Initialize database schema (create tables if needed)
	if err := s.initSchema(); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Load existing data from database into cache
	if err := s.loadFromDB(); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("load from DB: %w", err)
	}

	return s, nil
}

// initSchema creates the required database tables if they don't already exist.
// It creates two tables:
//   - gauge: For floating-point metrics with name as primary key
//   - counter: For integer counter metrics with name as primary key
//
// Returns:
//   - error: Any error during table creation
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

// loadFromDB populates the in-memory cache with existing data from the database.
// This is called during initialization to ensure the cache reflects the current
// database state.
//
// Returns:
//   - error: Any error during query execution or scanning
func (s *DBStorage) loadFromDB() error {
	// Load all gauge metrics
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

	// Load all counter metrics
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

// execWithRetry executes a database query with retry logic for transient errors.
// It uses a PostgresErrorClassifier to identify retriable errors (like connection issues,
// deadlocks, etc.) and retries with exponential backoff.
//
// Parameters:
//   - ctx: Context for the operation
//   - query: SQL query to execute
//   - args: Query arguments
//
// Returns:
//   - error: nil if successful, otherwise the last error encountered
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

// UpdateGauge updates or creates a gauge metric with the given name and value.
// The operation is atomic and updates both the database and in-memory cache.
//
// Parameters:
//   - name: Metric name
//   - value: New gauge value
//
// Returns:
//   - error: Any error during database operation
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

// UpdateCounter increments a counter metric by the given delta.
// The operation adds the delta to the existing value in the database and cache.
//
// Parameters:
//   - name: Metric name
//   - delta: Amount to increment by
//
// Returns:
//   - error: Any error during database operation
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

// SetCounter sets a counter metric to an absolute value.
// Unlike UpdateCounter which increments, this sets the exact value.
//
// Parameters:
//   - name: Metric name
//   - value: New absolute value
//
// Returns:
//   - error: Any error during database operation
func (s *DBStorage) SetCounter(name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO counter (name, value) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET value = $2`
	if err := s.execWithRetry(context.Background(), query, name, value); err != nil {
		return fmt.Errorf("set counter %s: %w", name, err)
	}
	s.cache.counter[name] = value
	return nil
}

// SaveCounterValue is an alias for SetCounter, provided for backward compatibility.
//
// Parameters:
//   - name: Metric name
//   - value: New absolute value
//
// Returns:
//   - error: Any error during database operation
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

// SaveAll persists all metrics from the in-memory cache to the database in a batch operation.
// This is useful for periodic backups or during shutdown.
//
// Returns:
//   - error: First error encountered during batch execution, or nil if successful
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

	// Create a batch operation for efficiency
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

	// Execute batch and collect errors
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

// Ping checks the database connection health.
//
// Parameters:
//   - ctx: Context for the ping operation
//
// Returns:
//   - error: nil if connection is healthy, otherwise connection error
func (s *DBStorage) Ping(ctx context.Context) error {
	return s.conn.PgConn().Ping(ctx)
}

// GetGauge retrieves a gauge metric value from the in-memory cache.
//
// Parameters:
//   - name: Metric name
//
// Returns:
//   - float64: The metric value
//   - bool: true if the metric exists, false otherwise
func (s *DBStorage) GetGauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetGauge(name)
}

// GetCounter retrieves a counter metric value from the in-memory cache.
//
// Parameters:
//   - name: Metric name
//
// Returns:
//   - int64: The metric value
//   - bool: true if the metric exists, false otherwise
func (s *DBStorage) GetCounter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetCounter(name)
}

// GetAll returns all metrics (both gauge and counter) from the in-memory cache.
//
// Returns:
//   - []metrics.Metrics: Slice of all metrics
//   - error: Always nil (kept for interface compatibility)
func (s *DBStorage) GetAll() ([]metrics.Metrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache.GetAll()
}

// Close closes the database connection.
//
// Returns:
//   - error: Any error during connection closure
func (s *DBStorage) Close() error {
	return s.conn.Close(context.Background())
}
