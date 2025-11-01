package wa

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// IdempotencyTTL is how long we keep idempotency records (24 hours)
	IdempotencyTTL = 24 * time.Hour
	// CleanupInterval is how often we clean up expired records
	CleanupInterval = 1 * time.Hour
)

type IdempotencyStore struct {
	records      map[string]*idempotencyRecord
	mu           sync.RWMutex
	cleanupTimer *time.Ticker
	stopChan     chan struct{}
}

type idempotencyRecord struct {
	messageID string
	timestamp time.Time
}

func NewIdempotencyStore() *IdempotencyStore {
	store := &IdempotencyStore{
		records:  make(map[string]*idempotencyRecord),
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	store.cleanupTimer = time.NewTicker(CleanupInterval)
	go store.cleanup()

	log.Info().Msg("Idempotency store initialized")

	return store
}

// CheckAndStore checks if a request with this idempotency key has been processed
// Returns (messageID, isDuplicate)
func (is *IdempotencyStore) CheckAndStore(idempotencyKey string, messageID string) (string, bool) {
	if idempotencyKey == "" {
		// No idempotency key provided, allow the request
		return "", false
	}

	is.mu.RLock()
	record, exists := is.records[idempotencyKey]
	is.mu.RUnlock()

	if exists {
		// Request already processed
		log.Debug().
			Str("idempotency_key", idempotencyKey).
			Str("original_message_id", record.messageID).
			Msg("Duplicate request detected")
		return record.messageID, true
	}

	// Store new record
	is.mu.Lock()
	is.records[idempotencyKey] = &idempotencyRecord{
		messageID: messageID,
		timestamp: time.Now(),
	}
	is.mu.Unlock()

	return "", false
}

func (is *IdempotencyStore) cleanup() {
	for {
		select {
		case <-is.cleanupTimer.C:
			is.performCleanup()
		case <-is.stopChan:
			is.cleanupTimer.Stop()
			return
		}
	}
}

func (is *IdempotencyStore) performCleanup() {
	is.mu.Lock()
	defer is.mu.Unlock()

	now := time.Now()
	threshold := now.Add(-IdempotencyTTL)
	cleaned := 0

	for key, record := range is.records {
		if record.timestamp.Before(threshold) {
			delete(is.records, key)
			cleaned++
		}
	}

	if cleaned > 0 {
		log.Info().
			Int("cleaned", cleaned).
			Int("remaining", len(is.records)).
			Msg("Idempotency store cleanup completed")
	}
}

func (is *IdempotencyStore) Stop() {
	close(is.stopChan)
	log.Info().Msg("Idempotency store stopped")
}

// GetStats returns statistics about the idempotency store
func (is *IdempotencyStore) GetStats() map[string]interface{} {
	is.mu.RLock()
	defer is.mu.RUnlock()

	return map[string]interface{}{
		"total_records": len(is.records),
		"ttl_hours":     IdempotencyTTL.Hours(),
	}
}
