package store

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type PostgresStore struct {
	db        *sql.DB
	container *sqlstore.Container
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	// Open database connection
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Create whatsmeow store container
	logger := waLog.Stdout("Database", "INFO", true)
	container := sqlstore.NewWithDB(db, "postgres", logger)

	// Run migrations
	if err := container.Upgrade(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info().Msg("Database store initialized successfully")

	return &PostgresStore{
		db:        db,
		container: container,
	}, nil
}

func (s *PostgresStore) GetDeviceStore(waAccountID string) (*store.Device, error) {
	// Use waAccountID as the store namespace
	device, err := s.container.GetDevice(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return device, nil
}

func (s *PostgresStore) GetAllDevices() ([]*store.Device, error) {
	devices, err := s.container.GetAllDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to get all devices: %w", err)
	}

	return devices, nil
}

func (s *PostgresStore) DeleteDevice(device *store.Device) error {
	if err := device.Delete(); err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	return nil
}

func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

func (s *PostgresStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Additional helper methods

func (s *PostgresStore) GetContainer() *sqlstore.Container {
	return s.container
}

func (s *PostgresStore) GetDB() *sql.DB {
	return s.db
}

// Execute custom queries if needed
func (s *PostgresStore) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *PostgresStore) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *PostgresStore) QueryRow(query string, args ...interface{}) *sql.Row {
	return s.db.QueryRow(query, args...)
}
