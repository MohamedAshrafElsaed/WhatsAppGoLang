// FILE: internal/handlers/group.go
// FIXES APPLIED:
// - Line 93: Added ctx parameter to GetJoinedGroups
// - Line 168: Added ctx parameter to CreateGroup
// - Line 215: Added ctx parameter to JoinGroupWithLink
// - Line 262: Added ctx parameter to GetGroupInfoFromLink
// - Line 306: Added ctx parameter to GetGroupInfo
// - Line 372: Added ctx parameter to UpdateGroupParticipants
// - Line 470: Added ctx parameter to SetGroupPhoto
// - Line 522: Added ctx parameter to SetGroupName
// - Line 568: Added ctx parameter to SetGroupLocked
// - Line 614: Added ctx parameter to SetGroupAnnounce
// - Line 660: Added ctx parameter to SetGroupTopic
// - Line 716: Added ctx parameter to GetGroupInviteLink
// - Line 762: Added ctx parameter to LeaveGroup
// - All group operations now have proper context handling
// VERIFICATION: All methods verified against doc.txt - all group methods require ctx as first parameter

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

	// Fixed: Added ctx parameter
	groups, err := mc.Client.GetJoinedGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get groups")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "groups_fetch_failed",
			"message":    "failed to get groups",
			"request_id": requestID,
		})
		return
	}

	groupList := []map[string]interface{}{}
	for _, group := range groups {
		groupList = append(groupList, map[string]interface{}{
			"jid":               group.JID.String(),
			"name":              group.Name,
			"owner":             group.OwnerJID.String(),
			"participant_count": len(group.Participants),
			"created_at":        group.GroupCreated,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":     groupList,
		"count":      len(groupList),
		"request_id": requestID,
	})
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	requestID := c.GetString("request_id")

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

	participants := []types.JID{}
	for _, p := range req.Participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			log.Error().Err(err).Str("participant", p).Msg("Failed to parse participant JID")
			continue
		}
		participants = append(participants, jid)
	}

	// Fixed: Added ctx parameter
	group, err := mc.Client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
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
		"success":    true,
		"group_id":   group.JID.String(),
		"name":       group.Name,
		"request_id": requestID,
	})
}

func (h *GroupHandler) JoinGroup(c *gin.Context) {
	var req JoinGroupRequest
	requestID := c.GetString("request_id")

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

	// Fixed: Added ctx parameter
	groupJID, err := mc.Client.JoinGroupWithLink(ctx, req.InviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to join group")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "join_failed",
			"message":    "failed to join group",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"group_id":   groupJID.String(),
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

	// Fixed: Added ctx parameter
	groupInfo, err := mc.Client.GetGroupInfoFromLink(ctx, inviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group preview")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "preview_failed",
			"message":    "failed to get group preview",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jid":               groupInfo.JID.String(),
		"name":              groupInfo.Name,
		"owner":             groupInfo.OwnerJID.String(),
		"participant_count": len(groupInfo.Participants),
		"created_at":        groupInfo.GroupCreated,
		"request_id":        requestID,
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

	// Fixed: Added ctx parameter
	groupInfo, err := mc.Client.GetGroupInfo(ctx, groupJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group info")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "group_info_failed",
			"message":    "failed to get group info",
			"request_id": requestID,
		})
		return
	}

	participants := []map[string]interface{}{}
	for _, p := range groupInfo.Participants {
		participants = append(participants, map[string]interface{}{
			"jid":            p.JID.String(),
			"is_admin":       p.IsAdmin,
			"is_super_admin": p.IsSuperAdmin,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"jid":          groupInfo.JID.String(),
		"name":         groupInfo.Name,
		"owner":        groupInfo.OwnerJID.String(),
		"topic":        groupInfo.Topic,
		"created_at":   groupInfo.GroupCreated,
		"participants": participants,
		"request_id":   requestID,
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
		jids := []types.JID{}
		for _, p := range req.Add {
			jid, err := types.ParseJID(p)
			if err != nil {
				log.Error().Err(err).Str("participant", p).Msg("Failed to parse participant JID")
				continue
			}
			jids = append(jids, jid)
		}
		// Fixed: Added ctx parameter
		addResult, err := mc.Client.UpdateGroupParticipants(ctx, groupJID, jids, whatsmeow.ParticipantChangeAdd)
		if err != nil {
			log.Error().Err(err).Msg("Failed to add participants")
			results["add_error"] = err.Error()
		} else {
			results["added"] = len(addResult)
		}
	}

	// Remove participants
	if len(req.Remove) > 0 {
		jids := []types.JID{}
		for _, p := range req.Remove {
			jid, err := types.ParseJID(p)
			if err != nil {
				log.Error().Err(err).Str("participant", p).Msg("Failed to parse participant JID")
				continue
			}
			jids = append(jids, jid)
		}
		// Fixed: Added ctx parameter
		removeResult, err := mc.Client.UpdateGroupParticipants(ctx, groupJID, jids, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			log.Error().Err(err).Msg("Failed to remove participants")
			results["remove_error"] = err.Error()
		} else {
			results["removed"] = len(removeResult)
		}
	}

	// Promote participants
	if len(req.Promote) > 0 {
		jids := []types.JID{}
		for _, p := range req.Promote {
			jid, err := types.ParseJID(p)
			if err != nil {
				log.Error().Err(err).Str("participant", p).Msg("Failed to parse participant JID")
				continue
			}
			jids = append(jids, jid)
		}
		// Fixed: Added ctx parameter
		promoteResult, err := mc.Client.UpdateGroupParticipants(ctx, groupJID, jids, whatsmeow.ParticipantChangePromote)
		if err != nil {
			log.Error().Err(err).Msg("Failed to promote participants")
			results["promote_error"] = err.Error()
		} else {
			results["promoted"] = len(promoteResult)
		}
	}

	// Demote participants
	if len(req.Demote) > 0 {
		jids := []types.JID{}
		for _, p := range req.Demote {
			jid, err := types.ParseJID(p)
			if err != nil {
				log.Error().Err(err).Str("participant", p).Msg("Failed to parse participant JID")
				continue
			}
			jids = append(jids, jid)
		}
		// Fixed: Added ctx parameter
		demoteResult, err := mc.Client.UpdateGroupParticipants(ctx, groupJID, jids, whatsmeow.ParticipantChangeDemote)
		if err != nil {
			log.Error().Err(err).Msg("Failed to demote participants")
			results["demote_error"] = err.Error()
		} else {
			results["demoted"] = len(demoteResult)
		}
	}

	results["success"] = true
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

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_open_failed",
			"message":    "failed to open photo file",
			"request_id": requestID,
		})
		return
	}
	defer src.Close()

	photoBytes, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "file_read_failed",
			"message":    "failed to read photo file",
			"request_id": requestID,
		})
		return
	}

	// Fixed: Added ctx parameter
	pictureID, err := mc.Client.SetGroupPhoto(ctx, groupJID, photoBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group photo")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "photo_update_failed",
			"message":    "failed to update group photo",
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

	// Fixed: Added ctx parameter
	err = mc.Client.SetGroupName(ctx, groupJID, req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group name")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "name_update_failed",
			"message":    "failed to update group name",
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

	// Fixed: Added ctx parameter
	err = mc.Client.SetGroupLocked(ctx, groupJID, req.Locked)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group locked")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "locked_update_failed",
			"message":    "failed to update group locked status",
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

	// Fixed: Added ctx parameter
	err = mc.Client.SetGroupAnnounce(ctx, groupJID, req.Announce)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group announce")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "announce_update_failed",
			"message":    "failed to update group announce status",
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

	// Fixed: Added ctx parameter
	err = mc.Client.SetGroupTopic(ctx, groupJID, "", "", topic)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group topic")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "topic_update_failed",
			"message":    "failed to update group topic",
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
	reset := c.DefaultQuery("reset", "false") == "true"
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

	// Fixed: Added ctx parameter
	link, err := mc.Client.GetGroupInviteLink(ctx, groupJID, reset)
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
		"success":     true,
		"invite_link": link,
		"request_id":  requestID,
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

	// Fixed: Added ctx parameter
	err = mc.Client.LeaveGroup(ctx, groupJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to leave group")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "leave_failed",
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
