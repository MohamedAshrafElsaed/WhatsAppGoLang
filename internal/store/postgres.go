package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
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

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool for high concurrency
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(1 * time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Create whatsmeow store container
	logger := waLog.Stdout("Database", "INFO", true)
	container := sqlstore.NewWithDB(db, "postgres", logger)

	// Run migrations with context
	upgradeCtx, upgradeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer upgradeCancel()

	if err := container.Upgrade(upgradeCtx); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info().Msg("Database store initialized successfully")

	return &PostgresStore{
		db:        db,
		container: container,
	}, nil
}

// GetDeviceStore returns a device store for a specific WhatsApp account
func (s *PostgresStore) GetDeviceStore(waAccountID string) (*store.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// For new devices, pass nil JID - whatsmeow will create a new device
	// The waAccountID is just used for logging/tracking on our end
	device, err := s.container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device for account %s: %w", waAccountID, err)
	}

	// If no device exists, GetFirstDevice returns a new one
	return device, nil
}

func (s *PostgresStore) GetAllDevices() ([]*store.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	devices, err := s.container.GetAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all devices: %w", err)
	}

	return devices, nil
}

func (s *PostgresStore) DeleteDevice(device *store.Device) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := device.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	return nil
}

func (s *PostgresStore) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.db.PingContext(ctx)
}

func (s *PostgresStore) Close() error {
	if s.db != nil {
		log.Info().Msg("Closing database connections")
		return s.db.Close()
	}
	return nil
}

// Helper methods

func (s *PostgresStore) GetContainer() *sqlstore.Container {
	return s.container
}

func (s *PostgresStore) GetDB() *sql.DB {
	return s.db
}

func (s *PostgresStore) Exec(query string, args ...interface{}) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.db.ExecContext(ctx, query, args...)
}

func (s *PostgresStore) Query(query string, args ...interface{}) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.db.QueryContext(ctx, query, args...)
}

func (s *PostgresStore) QueryRow(query string, args ...interface{}) *sql.Row {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.db.QueryRowContext(ctx, query, args...)
}

func (s *PostgresStore) GetDeviceByJID(jidStr string) (*store.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid JID: %w", err)
	}

	// Get device by JID
	devices, err := s.container.GetAllDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	// Find device with matching JID
	for _, device := range devices {
		if device.ID != nil && device.ID.String() == jid.String() {
			return device, nil
		}
	}

	return nil, fmt.Errorf("device not found for JID: %s", jidStr)
}

func (s *PostgresStore) PutDevice(device *store.Device) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return device.Save(ctx)
}
