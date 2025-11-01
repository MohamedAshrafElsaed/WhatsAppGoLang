package handlers

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"go.mau.fi/whatsmeow"
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
	requestID := c.GetString("request_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "phone and wa_account_id are required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	jid, err := types.ParseJID(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_phone",
			"message":    "invalid phone number format",
			"request_id": requestID,
		})
		return
	}

	// Get profile picture with proper parameters
	pic, err := mc.Client.GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
		Preview: false,
	})
	if err != nil {
		log.Error().Err(err).Str("phone", phone).Msg("Failed to get avatar")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "avatar_fetch_failed",
			"message":    "failed to get avatar",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        pic.URL,
		"id":         pic.ID,
		"type":       pic.Type,
		"direct":     pic.DirectPath,
		"request_id": requestID,
	})
}

func (h *AccountHandler) ChangeAvatar(c *gin.Context) {
	waAccountID := c.PostForm("wa_account_id")
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameter",
			"message":    "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_file",
			"message":    "avatar file is required",
			"request_id": requestID,
		})
		return
	}

	// Validate file size (max 5MB for profile pictures)
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "file_too_large",
			"message":    "avatar file must be less than 5MB",
			"request_id": requestID,
		})
		return
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

	// Read file
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_read_error",
			"message":    "failed to read file",
			"request_id": requestID,
		})
		return
	}
	defer fileContent.Close()

	avatarBytes, err := io.ReadAll(fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_read_error",
			"message":    "failed to read file content",
			"request_id": requestID,
		})
		return
	}

	// Set profile picture using correct method
	pictureID, err := mc.Client.SetProfilePicture(avatarBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set avatar")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "avatar_update_failed",
			"message":    "failed to set avatar",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"picture_id": pictureID,
		"success":    true,
		"request_id": requestID,
	})
}

func (h *AccountHandler) RemoveAvatar(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameter",
			"message":    "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	// Remove profile picture by setting empty bytes
	_, err = mc.Client.SetProfilePicture(nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove avatar")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "avatar_remove_failed",
			"message":    "failed to remove avatar",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "avatar removed successfully",
		"request_id": requestID,
	})
}

type ChangePushNameRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Name        string `json:"name" binding:"required,min=1,max=25"`
}

func (h *AccountHandler) ChangePushName(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req ChangePushNameRequest

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

	err = mc.Client.SetStatusMessage(req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to change push name")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "push_name_update_failed",
			"message":    "failed to change push name",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"name":       req.Name,
		"request_id": requestID,
	})
}

type SetStatusRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Status      string `json:"status" binding:"required,max=139"`
}

func (h *AccountHandler) SetStatus(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req SetStatusRequest

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

	err = mc.Client.SetStatusMessage(req.Status)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set status")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "status_update_failed",
			"message":    "failed to set status",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"status":     req.Status,
		"request_id": requestID,
	})
}

func (h *AccountHandler) GetUserInfo(c *gin.Context) {
	phone := c.Query("phone")
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "phone and wa_account_id are required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	parsedJID, err := types.ParseJID(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_phone",
			"message":    "invalid phone number format",
			"request_id": requestID,
		})
		return
	}

	jids := []types.JID{parsedJID}
	resp, err := mc.Client.GetUserInfo(jids)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user info")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "user_info_fetch_failed",
			"message":    "failed to get user info",
			"request_id": requestID,
		})
		return
	}

	if info, ok := resp[parsedJID]; ok {
		c.JSON(http.StatusOK, gin.H{
			"jid":         info.JID.String(),
			"verify_name": info.VerifiedName,
			"status":      info.Status,
			"picture_id":  info.PictureID,
			"devices":     info.Devices,
			"request_id":  requestID,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":      "user_not_found",
		"message":    "user not found",
		"request_id": requestID,
	})
}

func (h *AccountHandler) GetBusinessProfile(c *gin.Context) {
	jid := c.Query("jid")
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if jid == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "jid and wa_account_id are required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_jid",
			"message":    "invalid JID format",
			"request_id": requestID,
		})
		return
	}

	profile, err := mc.Client.GetBusinessProfile(parsedJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get business profile")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "business_profile_fetch_failed",
			"message":    "failed to get business profile",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jid":         profile.JID.String(),
		"email":       profile.Email,
		"website":     profile.Website,
		"category":    profile.Category,
		"description": profile.Description,
		"address":     profile.Address,
		"request_id":  requestID,
	})
}

func (h *AccountHandler) GetPrivacySettings(c *gin.Context) {
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameter",
			"message":    "wa_account_id is required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	settings, err := mc.Client.TryFetchPrivacySettings(false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get privacy settings")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "privacy_settings_fetch_failed",
			"message":    "failed to get privacy settings",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_add":     settings.GroupAdd,
		"last_seen":     settings.LastSeen,
		"status":        settings.Status,
		"profile":       settings.Profile,
		"read_receipts": settings.ReadReceipts,
		"online":        settings.Online,
		"request_id":    requestID,
	})
}

func (h *AccountHandler) CheckUserExists(c *gin.Context) {
	phone := c.Query("phone")
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if phone == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "phone and wa_account_id are required",
			"request_id": requestID,
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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

	// Check if user is on WhatsApp
	resp, err := mc.Client.IsOnWhatsApp([]string{phone})
	if err != nil {
		log.Error().Err(err).Msg("Failed to check user")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "user_check_failed",
			"message":    "failed to check user",
			"request_id": requestID,
		})
		return
	}

	if len(resp) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"phone":      phone,
			"exists":     resp[0].IsIn,
			"jid":        resp[0].JID.String(),
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"phone":      phone,
		"exists":     false,
		"request_id": requestID,
	})
}
