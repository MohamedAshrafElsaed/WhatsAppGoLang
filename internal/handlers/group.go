package handlers

import (
	"context"
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
	Subject      string   `json:"subject" binding:"required"`
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
	Name        string `json:"name" binding:"required"`
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

func (h *GroupHandler) ListGroups(c *gin.Context) {
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

	groups, err := mc.Client.GetJoinedGroups()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get groups")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get groups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"groups": groups,
		"count":  len(groups),
	})
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	// Parse participant JIDs
	participants := make([]types.JID, len(req.Participants))
	for i, p := range req.Participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid participant JID: " + p})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": groupInfo.JID.String(),
		"subject":  req.Subject,
		"created":  true,
	})
}

func (h *GroupHandler) JoinGroup(c *gin.Context) {
	var req JoinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	groupJID, err := mc.Client.JoinGroupWithLink(req.InviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to join group")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to join group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": groupJID.String(),
		"joined":   true,
	})
}

func (h *GroupHandler) GetGroupPreview(c *gin.Context) {
	inviteLink := c.Query("invite_link")
	waAccountID := c.Query("wa_account_id")

	if inviteLink == "" || waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invite_link and wa_account_id are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	groupInfo, err := mc.Client.GetGroupInfoFromLink(inviteLink)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group preview")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group preview"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":    groupInfo.JID.String(),
		"name":        groupInfo.Name,
		"owner":       groupInfo.OwnerJID.String(),
		"created_at":  groupInfo.GroupCreated,
		"size":        groupInfo.Size,
		"description": groupInfo.Topic,
	})
}

func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	groupID := c.Param("groupId")
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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	groupInfo, err := mc.Client.GetGroupInfo(groupJID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group info")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id":     groupInfo.JID.String(),
		"name":         groupInfo.Name,
		"owner":        groupInfo.OwnerJID.String(),
		"created_at":   groupInfo.GroupCreated,
		"topic":        groupInfo.Topic,
		"locked":       groupInfo.IsLocked,
		"announce":     groupInfo.IsAnnounce,
		"participants": len(groupInfo.Participants),
	})
}

func (h *GroupHandler) ManageParticipants(c *gin.Context) {
	groupID := c.Param("groupId")
	var req ManageParticipantsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, req.WaAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
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
		}
	}

	c.JSON(http.StatusOK, results)
}

func (h *GroupHandler) SetGroupPhoto(c *gin.Context) {
	groupID := c.Param("groupId")
	waAccountID := c.PostForm("wa_account_id")

	if waAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wa_account_id is required"})
		return
	}

	file, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "photo file is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	mc, err := h.clientManager.GetOrCreateClient(ctx, waAccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get client"})
		return
	}

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	// Read file
	fileContent, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}
	defer fileContent.Close()

	photoBytes := make([]byte, file.Size)
	_, err = fileContent.Read(photoBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file content"})
		return
	}

	// Set group photo
	pictureID, err := mc.Client.SetGroupPhoto(groupJID, photoBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group photo")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set group photo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"picture_id": pictureID,
		"success":    true,
	})
}

func (h *GroupHandler) SetGroupName(c *gin.Context) {
	groupID := c.Param("groupId")
	var req SetGroupNameRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	err = mc.Client.SetGroupName(groupJID, req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group name")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set group name"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *GroupHandler) SetGroupLocked(c *gin.Context) {
	groupID := c.Param("groupId")
	var req SetGroupLockedRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	err = mc.Client.SetGroupLocked(groupJID, req.Locked)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group locked")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set group locked"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "locked": req.Locked})
}

func (h *GroupHandler) SetGroupAnnounce(c *gin.Context) {
	groupID := c.Param("groupId")
	var req SetGroupAnnounceRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	err = mc.Client.SetGroupAnnounce(groupJID, req.Announce)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group announce")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set group announce"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "announce": req.Announce})
}

func (h *GroupHandler) SetGroupTopic(c *gin.Context) {
	groupID := c.Param("groupId")
	var req SetGroupTopicRequest

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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	topic := ""
	if req.Topic != nil {
		topic = *req.Topic
	}

	err = mc.Client.SetGroupTopic(groupJID, "", "", topic)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set group topic")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set group topic"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *GroupHandler) GetGroupInviteLink(c *gin.Context) {
	groupID := c.Param("groupId")
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

	groupJID, err := types.ParseJID(groupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group JID"})
		return
	}

	link, err := mc.Client.GetGroupInviteLink(groupJID, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group invite link")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group invite link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"invite_link": link,
	})
}
