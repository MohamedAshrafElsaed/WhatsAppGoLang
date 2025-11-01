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

type GroupHandler struct {
	clientManager *wa.ClientManager
}

func NewGroupHandler(cm *wa.ClientManager) *GroupHandler {
	return &GroupHandler{clientManager: cm}
}

type CreateGroupRequest struct {
	WaAccountID  string   `json:"wa_account_id" binding:"required"`
	Subject      string   `json:"subject" binding:"required,min=1,max=25"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

type JoinGroupRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	InviteLink  string `json:"invite_link" binding:"required"`
}

type ManageParticipantsRequest struct {
	WaAccountID string   `json:"wa_account_id" binding:"required"`
	Add         []string `json:"add"`
	Remove      []string `json:"remove"`
	Promote     []string `json:"promote"`
	Demote      []string `json:"demote"`
}

type SetGroupNameRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Name        string `json:"name" binding:"required,min=1,max=25"`
}

type SetGroupLockedRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Locked      bool   `json:"locked"`
}

type SetGroupAnnounceRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	Announce    bool   `json:"announce"`
}

type SetGroupTopicRequest struct {
	WaAccountID string  `json:"wa_account_id" binding:"required"`
	Topic       *string `json:"topic"`
}

type LeaveGroupRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
}

func (h *GroupHandler) ListGroups(c *gin.Context) {
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

	groups, err := mc.Client.GetJoinedGroups()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get groups")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "groups_fetch_failed",
			"message":    "failed to get groups",
			"request_id": requestID,
		})
		return
	}

	// Format groups list
	groupsList := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		groupsList = append(groupsList, map[string]interface{}{
			"jid": group.String(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":     groupsList,
		"count":      len(groupsList),
		"request_id": requestID,
	})
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req CreateGroupRequest

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

	// Parse participant JIDs
	participants := make([]types.JID, len(req.Participants))
	for i, p := range req.Participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "invalid_participant",
				"message":    "invalid participant JID: " + p,
				"request_id": requestID,
			})
			return
		}
		participants[i] = jid
	}

	// Create group
	groupInfo, err := mc.Client.CreateGroup(whatsmeow.ReqCreateGroup{
		Name:         req.Subject,
		Participants: participants,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create group")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_create_failed",
			"message":    "failed to create group",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":   groupInfo.JID.String(),
		"subject":    req.Subject,
		"created":    true,
		"request_id": requestID,
	})
}

func (h *GroupHandler) JoinGroup(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req JoinGroupRequest

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

	groupJID, err := mc.Client.JoinGroupWithLink(req.InviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to join group")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_join_failed",
			"message":    "failed to join group",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":   groupJID.String(),
		"joined":     true,
		"request_id": requestID,
	})
}

func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req LeaveGroupRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	err = mc.Client.LeaveGroup(groupJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to leave group")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_leave_failed",
			"message":    "failed to leave group",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"left":       true,
		"request_id": requestID,
	})
}

func (h *GroupHandler) GetGroupPreview(c *gin.Context) {
	inviteLink := c.Query("invite_link")
	waAccountID := c.Query("wa_account_id")
	requestID := c.GetString("request_id")

	if inviteLink == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_parameters",
			"message":    "invite_link and wa_account_id are required",
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

	groupInfo, err := mc.Client.GetGroupInfoFromLink(inviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group preview")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_preview_failed",
			"message":    "failed to get group preview",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":    groupInfo.JID.String(),
		"name":        groupInfo.Name,
		"owner":       groupInfo.OwnerJID.String(),
		"created_at":  groupInfo.GroupCreated.Unix(),
		"size":        groupInfo.Size,
		"description": groupInfo.Topic,
		"request_id":  requestID,
	})
}

func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	groupID := c.Param("groupId")
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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	groupInfo, err := mc.Client.GetGroupInfo(groupJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group info")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_info_failed",
			"message":    "failed to get group info",
			"request_id": requestID,
		})
		return
	}

	// Format participants
	participants := make([]map[string]interface{}, 0, len(groupInfo.Participants))
	for _, participant := range groupInfo.Participants {
		participants = append(participants, map[string]interface{}{
			"jid":            participant.JID.String(),
			"is_admin":       participant.IsAdmin,
			"is_super_admin": participant.IsSuperAdmin,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":          groupInfo.JID.String(),
		"name":              groupInfo.Name,
		"owner":             groupInfo.OwnerJID.String(),
		"created_at":        groupInfo.GroupCreated.Unix(),
		"topic":             groupInfo.Topic,
		"locked":            groupInfo.IsLocked,
		"announce":          groupInfo.IsAnnounce,
		"participants":      participants,
		"participant_count": len(participants),
		"request_id":        requestID,
	})
}

func (h *GroupHandler) ManageParticipants(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req ManageParticipantsRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	results := make(map[string]interface{})

	// Add participants
	if len(req.Add) > 0 {
		jids := make([]types.JID, len(req.Add))
		for i, p := range req.Add {
			jids[i], _ = types.ParseJID(p)
		}
		addResult, err := mc.Client.UpdateGroupParticipants(groupJID, jids, whatsmeow.ParticipantChangeAdd)
		if err == nil {
			results["added"] = addResult
		} else {
			results["add_error"] = err.Error()
		}
	}

	// Remove participants
	if len(req.Remove) > 0 {
		jids := make([]types.JID, len(req.Remove))
		for i, p := range req.Remove {
			jids[i], _ = types.ParseJID(p)
		}
		removeResult, err := mc.Client.UpdateGroupParticipants(groupJID, jids, whatsmeow.ParticipantChangeRemove)
		if err == nil {
			results["removed"] = removeResult
		} else {
			results["remove_error"] = err.Error()
		}
	}

	// Promote participants
	if len(req.Promote) > 0 {
		jids := make([]types.JID, len(req.Promote))
		for i, p := range req.Promote {
			jids[i], _ = types.ParseJID(p)
		}
		promoteResult, err := mc.Client.UpdateGroupParticipants(groupJID, jids, whatsmeow.ParticipantChangePromote)
		if err == nil {
			results["promoted"] = promoteResult
		} else {
			results["promote_error"] = err.Error()
		}
	}

	// Demote participants
	if len(req.Demote) > 0 {
		jids := make([]types.JID, len(req.Demote))
		for i, p := range req.Demote {
			jids[i], _ = types.ParseJID(p)
		}
		demoteResult, err := mc.Client.UpdateGroupParticipants(groupJID, jids, whatsmeow.ParticipantChangeDemote)
		if err == nil {
			results["demoted"] = demoteResult
		} else {
			results["demote_error"] = err.Error()
		}
	}

	results["request_id"] = requestID
	c.JSON(http.StatusOK, results)
}

func (h *GroupHandler) SetGroupPhoto(c *gin.Context) {
	groupID := c.Param("groupId")
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

	file, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "missing_file",
			"message":    "photo file is required",
			"request_id": requestID,
		})
		return
	}

	// Validate file size (max 5MB)
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "file_too_large",
			"message":    "photo must be less than 5MB",
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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
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

	photoBytes, err := io.ReadAll(fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_read_error",
			"message":    "failed to read file content",
			"request_id": requestID,
		})
		return
	}

	// Set group photo
	pictureID, err := mc.Client.SetGroupPhoto(groupJID, photoBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group photo")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "photo_set_failed",
			"message":    "failed to set group photo",
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

func (h *GroupHandler) SetGroupName(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req SetGroupNameRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	err = mc.Client.SetGroupName(groupJID, req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group name")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "name_set_failed",
			"message":    "failed to set group name",
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

func (h *GroupHandler) SetGroupLocked(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req SetGroupLockedRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	err = mc.Client.SetGroupLocked(groupJID, req.Locked)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group locked")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "locked_set_failed",
			"message":    "failed to set group locked",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"locked":     req.Locked,
		"request_id": requestID,
	})
}

func (h *GroupHandler) SetGroupAnnounce(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req SetGroupAnnounceRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	err = mc.Client.SetGroupAnnounce(groupJID, req.Announce)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group announce")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "announce_set_failed",
			"message":    "failed to set group announce",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"announce":   req.Announce,
		"request_id": requestID,
	})
}

func (h *GroupHandler) SetGroupTopic(c *gin.Context) {
	groupID := c.Param("groupId")
	requestID := c.GetString("request_id")
	var req SetGroupTopicRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	topic := ""
	if req.Topic != nil {
		topic = *req.Topic
	}

	err = mc.Client.SetGroupTopic(groupJID, "", "", topic)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group topic")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "topic_set_failed",
			"message":    "failed to set group topic",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"topic":      topic,
		"request_id": requestID,
	})
}

func (h *GroupHandler) GetGroupInviteLink(c *gin.Context) {
	groupID := c.Param("groupId")
	waAccountID := c.Query("wa_account_id")
	resetLink := c.Query("reset") == "true"
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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_group_id",
			"message":    "invalid group JID",
			"request_id": requestID,
		})
		return
	}

	link, err := mc.Client.GetGroupInviteLink(groupJID, resetLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group invite link")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "invite_link_failed",
			"message":    "failed to get group invite link",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"invite_link": link,
		"request_id":  requestID,
	})
}
