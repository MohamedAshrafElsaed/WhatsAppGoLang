package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"go.mau.fi/whatsmeow/types"
)

type ChatHandler struct {
	clientManager *wa.ClientManager
}

func NewChatHandler(cm *wa.ClientManager) *ChatHandler {
	return &ChatHandler{clientManager: cm}
}

type PinChatRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Pinned      bool   `json:"pinned"`
}

type MarkAsReadRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
}

type ArchiveChatRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Archived    bool   `json:"archived"`
}

type MuteChatRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Muted       bool   `json:"muted"`
	Duration    int64  `json:"duration"` // Duration in seconds, 0 for permanent
}

func (h *ChatHandler) ListChats(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
	search := c.Query("search")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameter",
			"message":    "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	// Get all contacts as a proxy for chats
	contacts, err := mc.Client.Store.Contacts.GetAllContacts()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get contacts")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "contacts_fetch_failed",
			"message":    "failed to get chats",
			"request_id": requestID,
		})
		return
	}

	// Filter by search if provided
	filteredChats := []map[string]interface{}{}
	for jid, contact := range contacts {
		if search != "" {
			searchLower := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(contact.FullName), searchLower) &&
				!strings.Contains(strings.ToLower(contact.PushName), searchLower) &&
				!strings.Contains(strings.ToLower(jid.String()), searchLower) {
				continue
			}
		}

		chatInfo := map[string]interface{}{
			"jid":       jid.String(),
			"name":      contact.FullName,
			"push_name": contact.PushName,
			"is_group":  jid.Server == types.GroupServer,
		}
		filteredChats = append(filteredChats, chatInfo)
	}

	// Simple pagination
	start := (page - 1) * perPage
	end := start + perPage
	if start > len(filteredChats) {
		start = len(filteredChats)
	}
	if end > len(filteredChats) {
		end = len(filteredChats)
	}

	paginatedChats := filteredChats[start:end]

	c.JSON(http.StatusOK, gin.H{
		"chats": paginatedChats,
		"meta": gin.H{
			"current_page": page,
			"per_page":     perPage,
			"total":        len(filteredChats),
			"total_pages":  (len(filteredChats) + perPage - 1) / perPage,
		},
		"request_id": requestID,
	})
}

func (h *ChatHandler) GetChatMessages(c *gin.Context) {
	chatID := c.Param("chatId")
	waAccountID := c.Query("wa_account_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameter",
			"message":    "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	if limit < 1 || limit > 100 {
		limit = 50
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_chat_id",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	// Note: whatsmeow doesn't provide direct message history API
	// This would require custom storage implementation
	c.JSON(http.StatusOK, gin.H{
		"chat_id":    chatJID.String(),
		"messages":   []interface{}{},
		"info":       "Message history requires custom storage implementation",
		"request_id": requestID,
	})
}

func (h *ChatHandler) PinChat(c *gin.Context) {
	chatID := c.Param("chatId")
	requestID := c.GetString("request_id")
	var req PinChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_request",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_chat_id",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	// Pin/unpin chat
	err = mc.Client.SetChatPin(chatJID, req.Pinned)
	if err != nil {
		log.Error().Err(err).Msg("Failed to pin chat")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "pin_failed",
			"message":    "failed to pin chat",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"pinned":     req.Pinned,
		"request_id": requestID,
	})
}

func (h *ChatHandler) MarkAsRead(c *gin.Context) {
	chatID := c.Param("chatId")
	requestID := c.GetString("request_id")
	var req MarkAsReadRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_request",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_chat_id",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	// Mark chat as read
	err = mc.Client.MarkRead([]types.MessageID{}, time.Now(), chatJID, types.EmptyJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to mark chat as read")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "mark_read_failed",
			"message":    "failed to mark as read",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"request_id": requestID,
	})
}

func (h *ChatHandler) ArchiveChat(c *gin.Context) {
	chatID := c.Param("chatId")
	requestID := c.GetString("request_id")
	var req ArchiveChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_request",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_chat_id",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	// Archive/unarchive chat
	err = mc.Client.SetChatArchive(chatJID, req.Archived, time.Time{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to archive chat")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "archive_failed",
			"message":    "failed to archive chat",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"archived":   req.Archived,
		"request_id": requestID,
	})
}

func (h *ChatHandler) MuteChat(c *gin.Context) {
	chatID := c.Param("chatId")
	requestID := c.GetString("request_id")
	var req MuteChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_request",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_error",
			"message":    "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "account not connected",
			"request_id": requestID,
		})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_chat_id",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	// Calculate mute expiry
	var muteExpiry time.Time
	if req.Muted {
		if req.Duration > 0 {
			muteExpiry = time.Now().Add(time.Duration(req.Duration) * time.Second)
		} else {
			// Permanent mute (8 hours from now as WhatsApp doesn't support permanent)
			muteExpiry = time.Now().Add(8 * 365 * 24 * time.Hour)
		}
	}

	// Mute/unmute chat
	err = mc.Client.SetChatMute(chatJID, req.Muted, muteExpiry)
	if err != nil {
		log.Error().Err(err).Msg("Failed to mute chat")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "mute_failed",
			"message":    "failed to mute chat",
			"request_id": requestID,
		})
		return
	}

	response := gin.H{
		"success":    true,
		"muted":      req.Muted,
		"request_id": requestID,
	}

	if req.Muted {
		response["mute_expiry"] = muteExpiry
	}

	c.JSON(http.StatusOK, response)
}
