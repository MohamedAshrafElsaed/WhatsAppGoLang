package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Sender struct {
	baseURL       string
	signingSecret string
	httpClient    *http.Client
}

func NewSender(baseURL, signingSecret string) *Sender {
	return &Sender{
		baseURL:       baseURL,
		signingSecret: signingSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type WebhookPayload struct {
	EventType   string                 `json:"event_type"`
	WaAccountID string                 `json:"wa_account_id"`
	TenantID    string                 `json:"tenant_id,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
	RequestID   string                 `json:"request_id"`
}

func (s *Sender) Send(endpoint string, payload WebhookPayload) error {
	if payload.RequestID == "" {
		payload.RequestID = uuid.New().String()
	}
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now()
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HMAC signature
	signature := s.sign(jsonData)

	// Build full URL
	url := fmt.Sprintf("%s/%s", s.baseURL, endpoint)

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WA-Signature", signature)
	req.Header.Set("X-Request-ID", payload.RequestID)

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", url).
			Str("event_type", payload.EventType).
			Msg("Failed to send webhook")
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Warn().
			Int("status", resp.StatusCode).
			Str("url", url).
			Str("event_type", payload.EventType).
			Msg("Webhook returned non-2xx status")
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Info().
		Str("url", url).
		Str("event_type", payload.EventType).
		Int("status", resp.StatusCode).
		Msg("Webhook sent successfully")

	return nil
}

func (s *Sender) sign(data []byte) string {
	h := hmac.New(sha256.New, []byte(s.signingSecret))
	h.Write(data)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// Convenience methods for specific webhook types

func (s *Sender) SendInbound(waAccountID, tenantID string, message map[string]interface{}) error {
	return s.Send("inbound", WebhookPayload{
		EventType:   "inbound",
		WaAccountID: waAccountID,
		TenantID:    tenantID,
		Data:        message,
	})
}

func (s *Sender) SendDelivery(waAccountID, tenantID, messageID string) error {
	return s.Send("delivery", WebhookPayload{
		EventType:   "delivery",
		WaAccountID: waAccountID,
		TenantID:    tenantID,
		Data: map[string]interface{}{
			"message_id": messageID,
			"status":     "delivered",
		},
	})
}

func (s *Sender) SendRead(waAccountID, tenantID, messageID string) error {
	return s.Send("read", WebhookPayload{
		EventType:   "read",
		WaAccountID: waAccountID,
		TenantID:    tenantID,
		Data: map[string]interface{}{
			"message_id": messageID,
		},
	})
}

func (s *Sender) SendStatus(waAccountID, status, message string) error {
	return s.Send("status", WebhookPayload{
		EventType:   "status",
		WaAccountID: waAccountID,
		Data: map[string]interface{}{
			"status":  status,
			"message": message,
		},
	})
}

func (s *Sender) SendError(waAccountID, tenantID, errorCode, errorMessage string, context map[string]interface{}) error {
	return s.Send("errors", WebhookPayload{
		EventType:   "error",
		WaAccountID: waAccountID,
		TenantID:    tenantID,
		Data: map[string]interface{}{
			"error_code":    errorCode,
			"error_message": errorMessage,
			"context":       context,
		},
	})
}
