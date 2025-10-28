package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

type SessionHandler struct {
	clientManager *wa.ClientManager
	webhookSender *webhooks.Sender
}

func NewSessionHandler(cm *wa.ClientManager, ws *webhooks.Sender) *SessionHandler {
	return &SessionHandler{
		clientManager: cm,
		webhookSender: ws,
	}
}

type QRResponse struct {
	QRCode       string    `json:"qr_code"` // Base64 PNG
	ExpiresAt    time.Time `json:"expires_at"`
	SessionState string    `json:"session_state"`
	RequestID    string    `json:"request_id"`
}

func (h *SessionHandler) GetQR(c *gin.Context) {
	waAccountID := c.Param("waAccountId")
	requestID := c.GetString("request_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to get client")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed_to_create_client",
			"message":    "Failed to initialize WhatsApp client",
			"request_id": requestID,
		})
		return
	}

	if mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "already_connected",
			"message":    "Account is already connected",
			"request_id": requestID,
		})
		return
	}

	// Channel to receive QR code
	qrChan, err := mc.Client.GetQRChannel(ctx)
	if err != nil {
		log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to get QR channel")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "qr_channel_failed",
			"message":    "Failed to initialize QR code generation",
			"request_id": requestID,
		})
		return
	}

	// Connect in background
	go func() {
		if err := mc.Client.Connect(); err != nil {
			log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to connect")
			h.webhookSender.SendStatus(waAccountID, "failed", err.Error())
		}
	}()

	// Setup event handler for connection
	eventHandler := mc.Client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Connected:
			mc.mu.Lock()
			mc.Connected = true
			mc.LastActivity = time.Now()
			mc.mu.Unlock()

			log.Info().Str("wa_account_id", waAccountID).Msg("WhatsApp connected")
			h.webhookSender.SendStatus(waAccountID, "connected", "")

		case *events.Disconnected:
			mc.mu.Lock()
			mc.Connected = false
			mc.mu.Unlock()

			log.Info().Str("wa_account_id", waAccountID).Msg("WhatsApp disconnected")
			h.webhookSender.SendStatus(waAccountID, "disconnected", "")

		case *events.LoggedOut:
			log.Info().Str("wa_account_id", waAccountID).Msg("WhatsApp logged out")
			h.webhookSender.SendStatus(waAccountID, "logged_out", "")
			h.clientManager.RemoveClient(waAccountID)
		}
	})
	defer mc.Client.RemoveEventHandler(eventHandler)

	// Wait for QR code
	select {
	case evt := <-qrChan:
		if evt.Event == "code" {
			// Generate PNG from QR code string
			qrCode, err := whatsmeow.GenerateQRCodeImage(evt.Code, 256, 256)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate QR image")
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "qr_generation_failed",
					"message":    "Failed to generate QR code image",
					"request_id": requestID,
				})
				return
			}

			// Encode to base64
			var buf bytes.Buffer
			if err := png.Encode(&buf, qrCode); err != nil {
				log.Error().Err(err).Msg("Failed to encode QR image")
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "qr_encoding_failed",
					"message":    "Failed to encode QR code",
					"request_id": requestID,
				})
				return
			}

			qrBase64 := base64.StdEncoding.EncodeToString(buf.Bytes())

			c.JSON(http.StatusOK, QRResponse{
				QRCode:       qrBase64,
				ExpiresAt:    time.Now().Add(60 * time.Second),
				SessionState: "pairing",
				RequestID:    requestID,
			})
			return

		} else if evt.Event == "success" {
			c.JSON(http.StatusOK, gin.H{
				"message":       "Successfully paired",
				"session_state": "connected",
				"request_id":    requestID,
			})
			return
		}

	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error":      "timeout",
			"message":    "QR code generation timeout",
			"request_id": requestID,
		})
		return
	}
}

type PairRequest struct {
	PairingCode string `json:"pairing_code" binding:"required"`
}

func (h *SessionHandler) PairWithCode(c *gin.Context) {
	waAccountID := c.Param("waAccountId")
	requestID := c.GetString("request_id")

	var req PairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_request",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to get client")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed_to_create_client",
			"message":    "Failed to initialize WhatsApp client",
			"request_id": requestID,
		})
		return
	}

	if mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "already_connected",
			"message":    "Account is already connected",
			"request_id": requestID,
		})
		return
	}

	// Get pairing code
	code, err := mc.Client.PairPhone(req.PairingCode, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to pair with code")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "pairing_failed",
			"message":    fmt.Sprintf("Failed to pair: %v", err),
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Pairing initiated, check your phone",
		"pairing_code":  code,
		"session_state": "pairing",
		"request_id":    requestID,
	})
}

func (h *SessionHandler) Reconnect(c *gin.Context) {
	waAccountID := c.Param("waAccountId")
	requestID := c.GetString("request_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed_to_get_client",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	if mc.Client.IsConnected() {
		c.JSON(http.StatusOK, gin.H{
			"message":    "Already connected",
			"request_id": requestID,
		})
		return
	}

	go func() {
		if err := mc.Client.Connect(); err != nil {
			log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to reconnect")
			h.webhookSender.SendStatus(waAccountID, "failed", err.Error())
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":    "Reconnection initiated",
		"request_id": requestID,
	})
}

func (h *SessionHandler) Logout(c *gin.Context) {
	waAccountID := c.Param("waAccountId")
	requestID := c.GetString("request_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed_to_get_client",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	if err := mc.Client.Logout(); err != nil {
		log.Error().Err(err).Str("wa_account_id", waAccountID).Msg("Failed to logout")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "logout_failed",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	h.clientManager.RemoveClient(waAccountID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "Successfully logged out",
		"request_id": requestID,
	})
}

func (h *SessionHandler) GetStatus(c *gin.Context) {
	waAccountID := c.Param("waAccountId")
	requestID := c.GetString("request_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"wa_account_id": waAccountID,
			"connected":     false,
			"state":         "not_initialized",
			"request_id":    requestID,
		})
		return
	}

	mc.mu.RLock()
	connected := mc.Connected
	lastActivity := mc.LastActivity
	mc.mu.RUnlock()

	state := "disconnected"
	if connected {
		state = "connected"
	} else if mc.Client.IsLoggedIn() {
		state = "logged_in"
	}

	c.JSON(http.StatusOK, gin.H{
		"wa_account_id": waAccountID,
		"connected":     connected,
		"state":         state,
		"last_activity": lastActivity,
		"request_id":    requestID,
	})
}
