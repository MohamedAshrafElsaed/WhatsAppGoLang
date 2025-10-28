package handlers

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"go.mau.fi/whatsmeow/types"
)

type AccountHandler struct {
	clientManager *wa.ClientManager
}

func NewAccountHandler(cm *wa.ClientManager) *AccountHandler {
	return &AccountHandler{clientManager: cm}
}

func (h *AccountHandler) GetAvatar(c *gin.Context) {
	phone := c.Query("phone")
	waAccountID := c.Query("wa_account_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone and wa_account_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	jid, err := types.ParseJID(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phone number"})
		return
	}

	pic, err := mc.Client.GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get avatar")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get avatar"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":    pic.URL,
		"id":     pic.ID,
		"type":   pic.Type,
		"direct": pic.DirectPath,
	})
}

func (h *AccountHandler) ChangeAvatar(c *gin.Context) {
	waAccountID := c.PostForm("wa_account_id")
	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wa_account_id is required"})
		return
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "avatar file is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	// Read file
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}
	defer fileContent.Close()

	avatarBytes, err := io.ReadAll(fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file content"})
		return
	}

	// Set profile picture
	pictureID, err := mc.Client.SetGroupPhoto(types.EmptyJID, avatarBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set avatar")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set avatar"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"picture_id": pictureID,
		"success":    true,
	})
}

type ChangePushNameRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
}

func (h *AccountHandler) ChangePushName(c *gin.Context) {
	var req ChangePushNameRequest
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

	err = mc.Client.SetStatusMessage(req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to change push name")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change push name"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"name":    req.Name,
	})
}

func (h *AccountHandler) GetUserInfo(c *gin.Context) {
	phone := c.Query("phone")
	waAccountID := c.Query("wa_account_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone and wa_account_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	jids := []types.JID{}
	parsedJID, err := types.ParseJID(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phone number"})
		return
	}
	jids = append(jids, parsedJID)

	resp, err := mc.Client.GetUserInfo(jids)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user info")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}

	if info, ok := resp[parsedJID]; ok {
		c.JSON(http.StatusOK, gin.H{
			"jid":         info.JID.String(),
			"verify_name": info.VerifiedName,
			"status":      info.Status,
			"picture_id":  info.PictureID,
			"devices":     info.Devices,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
}

func (h *AccountHandler) GetBusinessProfile(c *gin.Context) {
	jid := c.Query("jid")
	waAccountID := c.Query("wa_account_id")

	if jid == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jid and wa_account_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JID"})
		return
	}

	profile, err := mc.Client.GetBusinessProfile(parsedJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get business profile")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get business profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jid":         profile.JID.String(),
		"email":       profile.Email,
		"website":     profile.Website,
		"category":    profile.Category,
		"description": profile.Description,
		"address":     profile.Address,
	})
}

func (h *AccountHandler) GetPrivacySettings(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wa_account_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	settings, err := mc.Client.TryFetchPrivacySettings(false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get privacy settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get privacy settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_add":     settings.GroupAdd,
		"last_seen":     settings.LastSeen,
		"status":        settings.Status,
		"profile":       settings.Profile,
		"read_receipts": settings.ReadReceipts,
		"online":        settings.Online,
	})
}

func (h *AccountHandler) CheckUserExists(c *gin.Context) {
	phone := c.Query("phone")
	waAccountID := c.Query("wa_account_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone and wa_account_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	// Parse phone to JID
	parsedJID, err := types.ParseJID(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phone number"})
		return
	}

	// Check if user is on WhatsApp
	resp, err := mc.Client.IsOnWhatsApp([]string{phone})
	if err != nil {
		log.Error().Err(err).Msg("Failed to check user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check user"})
		return
	}

	if len(resp) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"phone":  phone,
			"exists": resp[0].IsIn,
			"jid":    resp[0].JID.String(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"phone":  phone,
		"exists": false,
		"jid":    parsedJID.String(),
	})
}
