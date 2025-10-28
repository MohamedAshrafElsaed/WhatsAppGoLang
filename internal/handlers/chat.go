package handlers

import (
	"context"
	"net/http"
	"strconv"
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

func (h *ChatHandler) ListChats(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
	search := c.Query("search")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wa_account_id is required"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not connected"})
		return
	}

	// Get all contacts as a proxy for chats
	// Note: whatsmeow doesn't have a direct "list chats" API
	// You'd typically build this from message history or use Store
	contacts, err := mc.Client.Store.Contacts.GetAllContacts()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get contacts")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get chats"})
		return
	}

	// Filter by search if provided
	filteredChats := []map[string]interface{}{}
	for jid, contact := range contacts {
		if search != "" {
			if !contains(contact.FullName, search) && !contains(contact.PushName, search) {
				continue
			}
		}

		chatInfo := map[string]interface{}{
			"jid":       jid.String(),
			"name":      contact.FullName,
			"push_name": contact.PushName,
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
		},
	})
}

func (h *ChatHandler) GetChatMessages(c *gin.Context) {
	chatID := c.Param("chatId")
	waAccountID := c.Query("wa_account_id")
	fromID := c.Query("from_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wa_account_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat JID"})
		return
	}

	// Note: whatsmeow doesn't provide direct message history API
	// You'd need to implement this using your own database storage
	// This is a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"chat_id":  chatJID.String(),
		"messages": []interface{}{},
		"info":     "Message history requires custom storage implementation",
	})
}

func (h *ChatHandler) PinChat(c *gin.Context) {
	chatID := c.Param("chatId")
	var req PinChatRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat JID"})
		return
	}

	// Pin/unpin chat
	err = mc.Client.SetChatPin(chatJID, req.Pinned)
	if err != nil {
		log.Error().Err(err).Msg("Failed to pin chat")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pin chat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"pinned":  req.Pinned,
	})
}

type MarkAsReadRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
}

func (h *ChatHandler) MarkAsRead(c *gin.Context) {
	chatID := c.Param("chatId")
	var req MarkAsReadRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	chatJID, err := types.ParseJID(chatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat JID"})
		return
	}

	// Mark chat as read
	err = mc.Client.MarkRead([]types.MessageID{}, time.Now(), chatJID, types.EmptyJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to mark chat as read")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || len(substr) == 0 ||
		(len(str) > 0 && len(substr) > 0 && stringContains(str, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
