// FILE: internal/handlers/newsletter.go
// FIXES APPLIED:
// - Line 42: Added ctx parameter to GetSubscribedNewsletters
// - Added proper error handling and context propagation
// - Fixed response structure to handle nil newsletter metadata properly
// VERIFICATION: GetSubscribedNewsletters(ctx) signature verified per doc.txt

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
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed to get client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "account not connected",
			"request_id": requestID,
		})
		return
	}

	// Fixed: Added ctx parameter
	newsletters, err := mc.Client.GetSubscribedNewsletters(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get newsletters")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "failed to get newsletters",
			"request_id": requestID,
		})
		return
	}

	newsletterList := []map[string]interface{}{}
	for _, newsletter := range newsletters {
		// Handle nil newsletter metadata safely
		if newsletter == nil {
			continue
		}

		item := map[string]interface{}{
			"id": newsletter.ID.String(),
		}

		// Safely access ThreadMeta fields
		if newsletter.ThreadMeta != nil {
			if newsletter.ThreadMeta.Name != nil {
				item["name"] = newsletter.ThreadMeta.Name.Text
			}
			if newsletter.ThreadMeta.Description != nil {
				item["description"] = newsletter.ThreadMeta.Description.Text
			}
			item["subscribers"] = newsletter.ThreadMeta.SubscriberCount
		}

		newsletterList = append(newsletterList, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"newsletters": newsletterList,
		"count":       len(newsletterList),
		"request_id":  requestID,
	})
}
