// FILE: internal/handlers/message.go
// FIXES APPLIED:
// - Line 24: Fixed import path from go.mau.fi/whatsmeow/binary/proto/waE2E to go.mau.fi/whatsmeow/proto/waE2E
// - Removed deprecated /binary/ from import path (whatsmeow API change)
// - Line 415: Added ctx parameter to SendPresence
// - Line 430: Added ctx parameter to SendChatPresence
// - All SendPresence calls now include proper context
// - Verified all media upload methods use correct context parameter
// - All message building methods properly propagate context
// VERIFICATION: Import path updated to match current whatsmeow version

package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MessageHandler struct {
	clientManager *wa.ClientManager
	webhookSender *webhooks.Sender
}

func NewMessageHandler(cm *wa.ClientManager, ws *webhooks.Sender) *MessageHandler {
	return &MessageHandler{
		clientManager: cm,
		webhookSender: ws,
	}
}

type SendMessageRequest struct {
	WaAccountID  string            `json:"wa_account_id" binding:"required"`
	To           string            `json:"to" binding:"required"`
	Type         string            `json:"type" binding:"required"`
	Body         string            `json:"body"`
	MediaURL     string            `json:"media_url"`
	FileName     string            `json:"file_name"`
	Mime         string            `json:"mime"`
	Caption      string            `json:"caption"`
	Image        *MediaInfo        `json:"image"`
	Video        *VideoInfo        `json:"video"`
	Audio        *AudioInfo        `json:"audio"`
	Document     *DocumentInfo     `json:"document"`
	Location     *LocationInfo     `json:"location"`
	Contact      *ContactInfo      `json:"contact"`
	Poll         *PollInfo         `json:"poll"`
	Link         *LinkInfo         `json:"link"`
	Presence     *PresenceInfo     `json:"presence"`
	ChatPresence *ChatPresenceInfo `json:"chat_presence"`
}

type MediaInfo struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

type VideoInfo struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

type AudioInfo struct {
	URL string `json:"url"`
	PTT bool   `json:"ptt"`
}

type DocumentInfo struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Mimetype string `json:"mimetype"`
}

type LocationInfo struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
}

type ContactInfo struct {
	Name   string   `json:"name"`
	Phones []string `json:"phones"`
	Org    string   `json:"org"`
}

type PollInfo struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type LinkInfo struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

type PresenceInfo struct {
	State string `json:"state"` // available, unavailable
}

type ChatPresenceInfo struct {
	JID   string `json:"jid"`
	State string `json:"state"` // typing, recording, paused
}

type DeleteMessageRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	ChatJID     string `json:"chat_jid" binding:"required"`
}

type RevokeMessageRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	ChatJID     string `json:"chat_jid" binding:"required"`
}

type ReactToMessageRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	ChatJID     string `json:"chat_jid" binding:"required"`
	Reaction    string `json:"reaction"`
}

type UpdateMessageRequest struct {
	WaAccountID string `json:"wa_account_id" binding:"required"`
	ChatJID     string `json:"chat_jid" binding:"required"`
	NewText     string `json:"new_text" binding:"required"`
}

func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req SendMessageRequest
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

	// Handle special message types
	if req.Type == "presence" {
		err := h.sendPresence(ctx, mc.Client, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":      "presence_failed",
				"message":    err.Error(),
				"request_id": requestID,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"type":       "presence",
			"request_id": requestID,
		})
		return
	}

	if req.Type == "chat_presence" {
		err := h.sendChatPresence(ctx, mc.Client, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":      "chat_presence_failed",
				"message":    err.Error(),
				"request_id": requestID,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"type":       "chat_presence",
			"request_id": requestID,
		})
		return
	}

	toJID, err := types.ParseJID(req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_recipient",
			"message":    "invalid recipient JID",
			"request_id": requestID,
		})
		return
	}

	var message *waE2E.Message

	switch req.Type {
	case "text":
		message = &waE2E.Message{
			Conversation: proto.String(req.Body),
		}
	case "image":
		message, err = h.buildImageMessage(ctx, mc.Client, req)
	case "video":
		message, err = h.buildVideoMessage(ctx, mc.Client, req)
	case "document":
		message, err = h.buildDocumentMessage(ctx, mc.Client, req)
	case "audio":
		message, err = h.buildAudioMessage(ctx, mc.Client, req)
	case "sticker":
		message, err = h.buildStickerMessage(ctx, mc.Client, req)
	case "location":
		message, err = h.buildLocationMessage(req)
	case "contact":
		message, err = h.buildContactMessage(req)
	case "poll":
		message, err = h.buildPollMessage(req)
	case "link":
		message, err = h.buildLinkMessage(req)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_message_type",
			"message":    "unsupported message type",
			"request_id": requestID,
		})
		return
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to build message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "message_build_failed",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	resp, err := mc.Client.SendMessage(ctx, toJID, message)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "send_failed",
			"message":    "failed to send message",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message_id": resp.ID,
		"timestamp":  resp.Timestamp,
		"request_id": requestID,
	})
}

func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	messageID := c.Param("messageId")
	requestID := c.GetString("request_id")
	var req DeleteMessageRequest

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

	chatJID, err := types.ParseJID(req.ChatJID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_jid",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	_, err = mc.Client.SendMessage(ctx, chatJID, &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key: &waE2E.MessageKey{
				FromMe:    proto.Bool(true),
				ID:        proto.String(messageID),
				RemoteJID: proto.String(chatJID.String()),
			},
		},
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "delete_failed",
			"message":    "failed to delete message",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "message deleted",
		"request_id": requestID,
	})
}

func (h *MessageHandler) RevokeMessage(c *gin.Context) {
	messageID := c.Param("messageId")
	requestID := c.GetString("request_id")
	var req RevokeMessageRequest

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

	chatJID, err := types.ParseJID(req.ChatJID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_jid",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	_, err = mc.Client.SendMessage(ctx, chatJID, &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key: &waE2E.MessageKey{
				FromMe:    proto.Bool(true),
				ID:        proto.String(messageID),
				RemoteJID: proto.String(chatJID.String()),
			},
		},
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to revoke message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "revoke_failed",
			"message":    "failed to revoke message",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "message revoked",
		"request_id": requestID,
	})
}

func (h *MessageHandler) ReactToMessage(c *gin.Context) {
	messageID := c.Param("messageId")
	requestID := c.GetString("request_id")
	var req ReactToMessageRequest

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

	chatJID, err := types.ParseJID(req.ChatJID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_jid",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	_, err = mc.Client.SendMessage(ctx, chatJID, &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waE2E.MessageKey{
				FromMe:    proto.Bool(true),
				ID:        proto.String(messageID),
				RemoteJID: proto.String(chatJID.String()),
			},
			Text:              proto.String(req.Reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to send reaction")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "reaction_failed",
			"message":    "failed to send reaction",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "reaction sent",
		"request_id": requestID,
	})
}

func (h *MessageHandler) UpdateMessage(c *gin.Context) {
	messageID := c.Param("messageId")
	requestID := c.GetString("request_id")
	var req UpdateMessageRequest

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

	chatJID, err := types.ParseJID(req.ChatJID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_jid",
			"message":    "invalid chat JID",
			"request_id": requestID,
		})
		return
	}

	_, err = mc.Client.SendMessage(ctx, chatJID, &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key: &waE2E.MessageKey{
				FromMe:    proto.Bool(true),
				ID:        proto.String(messageID),
				RemoteJID: proto.String(chatJID.String()),
			},
			EditedMessage: &waE2E.Message{
				Conversation: proto.String(req.NewText),
			},
		},
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to update message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "update_failed",
			"message":    "failed to update message",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "message updated",
		"request_id": requestID,
	})
}

func (h *MessageHandler) buildImageMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.MediaURL == "" {
		return nil, fmt.Errorf("media_url is required for image messages")
	}

	data, err := h.downloadMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(req.Mime),
		},
	}

	if req.Body != "" {
		msg.ImageMessage.Caption = proto.String(req.Body)
	}

	return msg, nil
}

func (h *MessageHandler) buildVideoMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.MediaURL == "" {
		return nil, fmt.Errorf("media_url is required for video messages")
	}

	data, err := h.downloadMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(req.Mime),
		},
	}

	if req.Body != "" {
		msg.VideoMessage.Caption = proto.String(req.Body)
	}

	return msg, nil
}

func (h *MessageHandler) buildDocumentMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.MediaURL == "" {
		return nil, fmt.Errorf("media_url is required for document messages")
	}

	data, err := h.downloadMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download document: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return nil, fmt.Errorf("failed to upload document: %w", err)
	}

	return &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			FileName:      proto.String(req.FileName),
			Mimetype:      proto.String(req.Mime),
		},
	}, nil
}

func (h *MessageHandler) buildAudioMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.Audio == nil || req.Audio.URL == "" {
		return nil, fmt.Errorf("audio data is required")
	}

	data, err := h.downloadMedia(ctx, req.Audio.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to download audio: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return nil, fmt.Errorf("failed to upload audio: %w", err)
	}

	return &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			PTT:           proto.Bool(req.Audio.PTT),
		},
	}, nil
}

func (h *MessageHandler) buildStickerMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.MediaURL == "" {
		return nil, fmt.Errorf("media_url is required for sticker messages")
	}

	data, err := h.downloadMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download sticker: %w", err)
	}

	uploaded, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload sticker: %w", err)
	}

	return &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String("image/webp"),
		},
	}, nil
}

func (h *MessageHandler) buildLocationMessage(req SendMessageRequest) (*waE2E.Message, error) {
	if req.Location == nil {
		return nil, fmt.Errorf("location data is required")
	}

	return &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(req.Location.Latitude),
			DegreesLongitude: proto.Float64(req.Location.Longitude),
			Name:             proto.String(req.Location.Name),
		},
	}, nil
}

func (h *MessageHandler) buildContactMessage(req SendMessageRequest) (*waE2E.Message, error) {
	if req.Contact == nil {
		return nil, fmt.Errorf("contact data is required")
	}

	vcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nFN:%s\n", req.Contact.Name)
	for _, phone := range req.Contact.Phones {
		vcard += fmt.Sprintf("TEL:%s\n", phone)
	}
	if req.Contact.Org != "" {
		vcard += fmt.Sprintf("ORG:%s\n", req.Contact.Org)
	}
	vcard += "END:VCARD"

	return &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(req.Contact.Name),
			Vcard:       proto.String(vcard),
		},
	}, nil
}

func (h *MessageHandler) buildPollMessage(req SendMessageRequest) (*waE2E.Message, error) {
	if req.Poll == nil {
		return nil, fmt.Errorf("poll data is required")
	}

	options := make([]*waE2E.PollCreationMessage_Option, len(req.Poll.Options))
	for i, opt := range req.Poll.Options {
		options[i] = &waE2E.PollCreationMessage_Option{
			OptionName: proto.String(opt),
		}
	}

	return &waE2E.Message{
		PollCreationMessage: &waE2E.PollCreationMessage{
			Name:                   proto.String(req.Poll.Question),
			Options:                options,
			SelectableOptionsCount: proto.Uint32(1),
		},
	}, nil
}

func (h *MessageHandler) buildLinkMessage(req SendMessageRequest) (*waE2E.Message, error) {
	if req.Link == nil || req.Link.URL == "" {
		return nil, fmt.Errorf("link data is required")
	}

	text := req.Link.URL
	if req.Link.Caption != "" {
		text = req.Link.Caption + "\n" + req.Link.URL
	}

	return &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:         proto.String(text),
			CanonicalURL: proto.String(req.Link.URL),
			MatchedText:  proto.String(req.Link.URL),
		},
	}, nil
}

// Fixed: Added ctx parameter to SendPresence
func (h *MessageHandler) sendPresence(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) error {
	if req.Presence == nil {
		return fmt.Errorf("presence data is required")
	}

	var presence types.Presence
	if req.Presence.State == "available" {
		presence = types.PresenceAvailable
	} else {
		presence = types.PresenceUnavailable
	}

	// Fixed: Added ctx as first parameter
	return client.SendPresence(ctx, presence)
}

// Fixed: Added ctx parameter to SendChatPresence
func (h *MessageHandler) sendChatPresence(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) error {
	if req.ChatPresence == nil {
		return fmt.Errorf("chat_presence data is required")
	}

	jid, err := types.ParseJID(req.ChatPresence.JID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	var state types.ChatPresence
	if req.ChatPresence.State == "typing" {
		state = types.ChatPresenceComposing
	} else if req.ChatPresence.State == "recording" {
		state = types.ChatPresenceComposing
	} else {
		state = types.ChatPresencePaused
	}

	// Fixed: Added ctx as first parameter
	return client.SendChatPresence(ctx, jid, state, types.ChatPresenceMediaText)
}

func (h *MessageHandler) downloadMedia(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download media: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
