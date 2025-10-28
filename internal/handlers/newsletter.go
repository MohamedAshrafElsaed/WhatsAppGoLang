package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
)

type NewsletterHandler struct {
	clientManager *wa.ClientManager
}

func NewNewsletterHandler(cm *wa.ClientManager) *NewsletterHandler {
	return &NewsletterHandler{clientManager: cm}
}

func (h *NewsletterHandler) ListNewsletters(c *gin.Context) {
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

	newsletters, err := mc.Client.GetSubscribedNewsletters()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get newsletters")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get newsletters"})
		return
	}

	newsletterList := []map[string]interface{}{}
	for _, newsletter := range newsletters {
		newsletterList = append(newsletterList, map[string]interface{}{
			"id":          newsletter.ID.String(),
			"name":        newsletter.ThreadMeta.Name.Text,
			"description": newsletter.ThreadMeta.Description.Text,
			"subscribers": newsletter.ThreadMeta.SubscriberCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"newsletters": newsletterList,
		"count":       len(newsletterList),
	})
}
