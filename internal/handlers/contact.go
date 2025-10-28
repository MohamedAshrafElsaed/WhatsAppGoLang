package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
)

type ContactHandler struct {
	clientManager *wa.ClientManager
}

func NewContactHandler(cm *wa.ClientManager) *ContactHandler {
	return &ContactHandler{clientManager: cm}
}

func (h *ContactHandler) GetContacts(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
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

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not connected"})
		return
	}

	contacts, err := mc.Client.Store.Contacts.GetAllContacts()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get contacts")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get contacts"})
		return
	}

	contactList := []map[string]interface{}{}
	for jid, contact := range contacts {
		contactList = append(contactList, map[string]interface{}{
			"jid":        jid.String(),
			"name":       contact.FullName,
			"push_name":  contact.PushName,
			"first_name": contact.FirstName,
			"business":   contact.BusinessName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"contacts": contactList,
		"count":    len(contactList),
	})
}
