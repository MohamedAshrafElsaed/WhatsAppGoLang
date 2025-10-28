package wa

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/config"
	"github.com/whatsapp-api/go-whatsapp-service/internal/store"
	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type ClientManager struct {
	clients  map[string]*ManagedClient
	mu       sync.RWMutex
	store    *store.PostgresStore
	config   *config.Config
	stopChan chan struct{}
	wg       sync.WaitGroup
}

type ManagedClient struct {
	Client       *whatsmeow.Client
	WaAccountID  string
	LastActivity time.Time
	Connected    bool
	mu           sync.RWMutex
}

func NewClientManager(store *store.PostgresStore, cfg *config.Config) *ClientManager {
	cm := &ClientManager{
		clients:  make(map[string]*ManagedClient),
		store:    store,
		config:   cfg,
		stopChan: make(chan struct{}),
	}

	// Start idle session cleanup goroutine
	cm.wg.Add(1)
	go cm.cleanupIdleSessions()

	return cm
}

func (cm *ClientManager) GetOrCreateClient(ctx context.Context, waAccountID string) (*ManagedClient, error) {
	cm.mu.RLock()
	if mc, exists := cm.clients[waAccountID]; exists {
		mc.mu.Lock()
		mc.LastActivity = time.Now()
		mc.mu.Unlock()
		cm.mu.RUnlock()
		return mc, nil
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Double-check after acquiring write lock
	if mc, exists := cm.clients[waAccountID]; exists {
		mc.mu.Lock()
		mc.LastActivity = time.Now()
		mc.mu.Unlock()
		return mc, nil
	}

	// Create new client
	deviceStore, err := cm.store.GetDeviceStore(waAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device store: %w", err)
	}

	device, err := deviceStore.GetDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	logger := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(device, logger)

	mc := &ManagedClient{
		Client:       client,
		WaAccountID:  waAccountID,
		LastActivity: time.Now(),
		Connected:    false,
	}

	cm.clients[waAccountID] = mc

	log.Info().Str("wa_account_id", waAccountID).Msg("Created new WhatsApp client")

	return mc, nil
}

func (cm *ClientManager) RemoveClient(waAccountID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if mc, exists := cm.clients[waAccountID]; exists {
		mc.mu.Lock()
		if mc.Connected {
			mc.Client.Disconnect()
		}
		mc.mu.Unlock()
		delete(cm.clients, waAccountID)
		log.Info().Str("wa_account_id", waAccountID).Msg("Removed WhatsApp client")
	}
}

func (cm *ClientManager) cleanupIdleSessions() {
	defer cm.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.performCleanup()
		case <-cm.stopChan:
			return
		}
	}
}

func (cm *ClientManager) performCleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	idleThreshold := now.Add(-cm.config.SessionIdleTTL)

	for waAccountID, mc := range cm.clients {
		mc.mu.RLock()
		isIdle := mc.LastActivity.Before(idleThreshold)
		isConnected := mc.Connected
		mc.mu.RUnlock()

		if isIdle && isConnected {
			log.Info().
				Str("wa_account_id", waAccountID).
				Time("last_activity", mc.LastActivity).
				Msg("Disconnecting idle session")

			mc.mu.Lock()
			mc.Client.Disconnect()
			mc.Connected = false
			mc.mu.Unlock()
		}
	}
}

func (cm *ClientManager) DisconnectAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for waAccountID, mc := range cm.clients {
		mc.mu.Lock()
		if mc.Connected {
			mc.Client.Disconnect()
			mc.Connected = false
			log.Info().Str("wa_account_id", waAccountID).Msg("Disconnected client during shutdown")
		}
		mc.mu.Unlock()
	}

	close(cm.stopChan)
	cm.wg.Wait()
}

func (cm *ClientManager) GetClientCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.clients)
}

func (cm *ClientManager) GetConnectedCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	count := 0
	for _, mc := range cm.clients {
		mc.mu.RLock()
		if mc.Connected {
			count++
		}
		mc.mu.RUnlock()
	}
	return count
}
