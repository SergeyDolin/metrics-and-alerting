package storage

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver initialization
	"github.com/pressly/goose/v3"      // Database migration tool
)

// RunMigrations executes database schema migrations using the goose migration tool.
// It applies all pending migrations from the "migrations" directory to bring the
// database schema up to date with the current version of the application.
//
// Migration files should be placed in the "./migrations" directory relative to
// where the application is run, and should follow goose naming conventions:
//   - 00001_create_tables.sql
//   - 00002_add_indexes.sql
//   - etc.
//
// The function performs the following steps:
//  1. Disables goose's default logging (sets to NopLogger)
//  2. Sets the database dialect to "postgres"
//  3. Runs all up migrations that haven't been applied yet
//
// Parameters:
//   - db: A pointer to an open sql.DB connection to the PostgreSQL database
//
// Returns:
//   - error: nil if migrations run successfully or no migrations are pending,
//     otherwise an error describing what went wrong (e.g., migration file errors,
//     SQL syntax errors, connection issues)
//
// Example usage:
//
//	db, err := sql.Open("pgx", "postgres://localhost/metrics")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	if err := RunMigrations(db); err != nil {
//	    log.Fatalf("Migration failed: %v", err)
//	}
//
// Note: This function assumes the database connection is already established
// and that the migrations directory exists and contains valid SQL migration files.
// The function will panic if goose encounters fatal errors during migration execution.
func RunMigrations(db *sql.DB) error {
	// Disable goose's default logging to prevent cluttering application logs
	// with migration progress messages (can be enabled for debugging if needed)
	goose.SetLogger(goose.NopLogger())

	// Set the database dialect to PostgreSQL
	// This ensures goose uses PostgreSQL-specific SQL syntax for tracking migrations
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// Apply all pending up migrations from the "migrations" directory
	// This will create the goose_db_version table if it doesn't exist
	// and run any new migration files in order
	return goose.Up(db, "migrations")
}
