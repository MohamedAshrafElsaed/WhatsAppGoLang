package wa

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/config"
	"github.com/whatsapp-api/go-whatsapp-service/internal/store"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type ClientManager struct {
	clients       map[string]*ManagedClient
	mu            sync.RWMutex
	store         *store.PostgresStore
	config        *config.Config
	webhookSender *webhooks.Sender
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

type ManagedClient struct {
	Client       *whatsmeow.Client
	WaAccountID  string
	LastActivity time.Time
	Connected    bool
	mu           sync.RWMutex
}

func NewClientManager(store *store.PostgresStore, cfg *config.Config, webhookSender *webhooks.Sender) *ClientManager {
	cm := &ClientManager{
		clients:       make(map[string]*ManagedClient),
		store:         store,
		config:        cfg,
		webhookSender: webhookSender,
		stopChan:      make(chan struct{}),
	}

	// Start idle session cleanup goroutine
	cm.wg.Add(1)
	go cm.cleanupIdleSessions()

	log.Info().Msg("Client manager initialized with webhook integration")

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
	device, err := cm.store.GetDeviceStore(waAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device store: %w", err)
	}

	logger := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(device, logger)

	mc := &ManagedClient{
		Client:       client,
		WaAccountID:  waAccountID,
		LastActivity: time.Now(),
		Connected:    false,
	}

	// Setup event handlers for this client
	SetupEventHandlers(mc, cm.webhookSender)

	cm.clients[waAccountID] = mc

	log.Info().
		Str("wa_account_id", waAccountID).
		Msg("Created new WhatsApp client with event handlers")

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

	log.Info().Msg("Started idle session cleanup goroutine")

	for {
		select {
		case <-ticker.C:
			cm.performCleanup()
		case <-cm.stopChan:
			log.Info().Msg("Stopping idle session cleanup goroutine")
			return
		}
	}
}

func (cm *ClientManager) performCleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	idleThreshold := now.Add(-cm.config.SessionIdleTTL)
	disconnectedCount := 0

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

			disconnectedCount++
		}
	}

	if disconnectedCount > 0 {
		log.Info().
			Int("disconnected", disconnectedCount).
			Int("total_clients", len(cm.clients)).
			Msg("Idle session cleanup completed")
	}
}

func (cm *ClientManager) DisconnectAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	log.Info().
		Int("total_clients", len(cm.clients)).
		Msg("Disconnecting all WhatsApp clients")

	for waAccountID, mc := range cm.clients {
		mc.mu.Lock()
		if mc.Connected {
			mc.Client.Disconnect()
			mc.Connected = false
			log.Debug().Str("wa_account_id", waAccountID).Msg("Disconnected client")
		}
		mc.mu.Unlock()
	}

	close(cm.stopChan)
	cm.wg.Wait()

	log.Info().Msg("All clients disconnected successfully")
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
