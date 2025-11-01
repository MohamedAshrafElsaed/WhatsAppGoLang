// FILE: internal/handlers/account.go
// FIXES APPLIED:
// - Line 69: Added ctx parameter to GetProfilePictureInfo
// - Line 243: Added ctx parameter to GetUserInfo
// - Line 460: Removed non-existent profile.Description field
// - Line 504: Only return available BusinessProfile fields (JID, Email, Address)
// - Line 564: Removed non-existent settings.Disappearing field
// - Line 548: Added ctx parameter to GetPrivacySettings
// - All methods now have proper context handling and error logging
// VERIFICATION: All methods verified against doc.txt sections for GetBusinessProfile, GetPrivacySettings, GetProfilePictureInfo

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

	// Fixed: Added ctx parameter
	pic, err := mc.Client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{
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

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_open_failed",
			"message":    "failed to open avatar file",
			"request_id": requestID,
		})
		return
	}
	defer src.Close()

	avatarBytes, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_read_failed",
			"message":    "failed to read avatar file",
			"request_id": requestID,
		})
		return
	}

	// Fixed: Added ctx parameter
	pictureID, err := mc.Client.SetProfilePicture(ctx, avatarBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set avatar")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "avatar_update_failed",
			"message":    "failed to update avatar",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"picture_id": pictureID,
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

	// Fixed: Added ctx parameter and pass nil to remove avatar
	_, err = mc.Client.SetProfilePicture(ctx, nil)
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
		"removed":    true,
		"request_id": requestID,
	})
}

func (h *AccountHandler) ChangePushName(c *gin.Context) {
	var req struct {
		WaAccountID string `json:"wa_account_id" binding:"required"`
		PushName    string `json:"push_name" binding:"required"`
	}

	requestID := c.GetString("request_id")

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

	// Fixed: Added ctx parameter
	err = mc.Client.SetPushName(ctx, req.PushName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set push name")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "push_name_update_failed",
			"message":    "failed to update push name",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"push_name":  req.PushName,
		"request_id": requestID,
	})
}

func (h *AccountHandler) SetStatus(c *gin.Context) {
	var req struct {
		WaAccountID string `json:"wa_account_id" binding:"required"`
		Status      string `json:"status" binding:"required"`
	}

	requestID := c.GetString("request_id")

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

	// Fixed: Added ctx parameter
	err = mc.Client.SetStatusMessage(ctx, req.Status)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set status")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "status_update_failed",
			"message":    "failed to update status",
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
	// Fixed: Added ctx parameter
	resp, err := mc.Client.GetUserInfo(ctx, jids)
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
			"jid":         parsedJID.String(),
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

	// Fixed: Added ctx parameter
	profile, err := mc.Client.GetBusinessProfile(ctx, parsedJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get business profile")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "business_profile_fetch_failed",
			"message":    "failed to get business profile",
			"request_id": requestID,
		})
		return
	}

	// Fixed: Only return fields that actually exist in types.BusinessProfile
	// According to doc.txt and whatsmeow source, BusinessProfile has: JID, Email, Address
	// Description field does not exist in the actual struct
	c.JSON(http.StatusOK, gin.H{
		"jid":        profile.JID.String(),
		"email":      profile.Email,
		"address":    profile.Address,
		"request_id": requestID,
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

	// Fixed: Added ctx parameter
	settings := mc.Client.GetPrivacySettings(ctx)

	// Fixed: Removed settings.Disappearing as it doesn't exist in types.PrivacySettings
	// According to doc.txt, PrivacySettings contains: GroupAdd, LastSeen, Status, Profile, ReadReceipts, CallAdd
	c.JSON(http.StatusOK, gin.H{
		"group_add":     settings.GroupAdd,
		"last_seen":     settings.LastSeen,
		"status":        settings.Status,
		"profile":       settings.Profile,
		"read_receipts": settings.ReadReceipts,
		"call_add":      settings.CallAdd,
		"request_id":    requestID,
	})
}

func (h *AccountHandler) CheckUserExists(c *gin.Context) {
	phones := c.QueryArray("phones")
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if len(phones) == 0 || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "phones and wa_account_id are required",
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

	// Fixed: Added ctx parameter
	results, err := mc.Client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check user existence")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "check_failed",
			"message":    "failed to check user existence",
			"request_id": requestID,
		})
		return
	}

	response := []map[string]interface{}{}
	for _, result := range results {
		response = append(response, map[string]interface{}{
			"phone":         result.Query,
			"exists":        result.IsIn,
			"jid":           result.JID.String(),
			"verified_name": result.VerifiedName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results":    response,
		"request_id": requestID,
	})
}
