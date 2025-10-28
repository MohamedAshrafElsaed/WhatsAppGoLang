package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
	WaAccountID    string            `json:"wa_account_id" binding:"required"`
	Type           string            `json:"type" binding:"required"`
	To             string            `json:"to" binding:"required"`
	Body           string            `json:"body"`
	MediaURL       string            `json:"media_url"`
	FileName       string            `json:"file_name"`
	Mime           string            `json:"mime"`
	Location       *LocationData     `json:"location"`
	Contact        *ContactData      `json:"contact"`
	Audio          *AudioData        `json:"audio"`
	Poll           *PollData         `json:"poll"`
	Presence       *PresenceData     `json:"presence"`
	ChatPresence   *ChatPresenceData `json:"chat_presence"`
	Link           *LinkData         `json:"link"`
	Vars           []string          `json:"vars"`
	CallbackURL    string            `json:"callback_url"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type LocationData struct {
	Latitude  float64 `json:"lat" binding:"required"`
	Longitude float64 `json:"lng" binding:"required"`
	Name      string  `json:"name"`
}

type ContactData struct {
	Name   string   `json:"name" binding:"required"`
	Phones []string `json:"phones" binding:"required"`
	Org    string   `json:"org"`
}

type AudioData struct {
	URL string `json:"url" binding:"required"`
	PTT bool   `json:"ptt"`
}

type PollData struct {
	Question string   `json:"question" binding:"required"`
	Options  []string `json:"options" binding:"required,min=2,max=12"`
}

type PresenceData struct {
	State string `json:"state" binding:"required,oneof=available unavailable"`
}

type ChatPresenceData struct {
	JID   string `json:"jid" binding:"required"`
	State string `json:"state" binding:"required,oneof=typing paused"`
}

type LinkData struct {
	URL     string `json:"url" binding:"required"`
	Caption string `json:"caption"`
}

type SendMessageResponse struct {
	MessageID string    `json:"message_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`
}

func (h *MessageHandler) SendMessage(c *gin.Context) {
	requestID := c.GetString("request_id")
	var req SendMessageRequest

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
		log.Error().Err(err).Str("wa_account_id", req.WaAccountID).Msg("Failed to get client")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "client_not_available",
			"message":    "Failed to get WhatsApp client",
			"request_id": requestID,
		})
		return
	}

	if !mc.Client.IsConnected() {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "not_connected",
			"message":    "WhatsApp account is not connected",
			"request_id": requestID,
		})
		return
	}

	// Parse recipient JID
	recipientJID, err := types.ParseJID(req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid_recipient",
			"message":    fmt.Sprintf("Invalid recipient JID: %v", err),
			"request_id": requestID,
		})
		return
	}

	// Build message based on type
	var msg *waE2E.Message
	var sendErr error

	switch req.Type {
	case "message":
		msg = &waE2E.Message{
			Conversation: proto.String(req.Body),
		}

	case "image":
		msg, sendErr = h.buildImageMessage(ctx, mc.Client, req)

	case "video":
		msg, sendErr = h.buildVideoMessage(ctx, mc.Client, req)

	case "file", "document":
		msg, sendErr = h.buildDocumentMessage(ctx, mc.Client, req)

	case "audio":
		msg, sendErr = h.buildAudioMessage(ctx, mc.Client, req)

	case "sticker":
		msg, sendErr = h.buildStickerMessage(ctx, mc.Client, req)

	case "location":
		msg, sendErr = h.buildLocationMessage(req)

	case "contact":
		msg, sendErr = h.buildContactMessage(req)

	case "poll":
		msg, sendErr = h.buildPollMessage(req)

	case "link":
		msg, sendErr = h.buildLinkMessage(req)

	case "presence":
		sendErr = h.sendPresence(mc.Client, req)
		if sendErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"status":     "sent",
				"request_id": requestID,
			})
			return
		}

	case "chat_presence":
		sendErr = h.sendChatPresence(mc.Client, req)
		if sendErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"status":     "sent",
				"request_id": requestID,
			})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "unsupported_type",
			"message":    fmt.Sprintf("Message type '%s' is not supported", req.Type),
			"request_id": requestID,
		})
		return
	}

	if sendErr != nil {
		log.Error().Err(sendErr).Str("type", req.Type).Msg("Failed to build message")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "message_build_failed",
			"message":    sendErr.Error(),
			"request_id": requestID,
		})
		return
	}

	// Send message
	resp, err := mc.Client.SendMessage(ctx, recipientJID, msg)
	if err != nil {
		log.Error().Err(err).Str("wa_account_id", req.WaAccountID).Msg("Failed to send message")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "send_failed",
			"message":    err.Error(),
			"request_id": requestID,
		})
		return
	}

	mc.mu.Lock()
	mc.LastActivity = time.Now()
	mc.mu.Unlock()

	c.JSON(http.StatusOK, SendMessageResponse{
		MessageID: resp.ID,
		Status:    "sent",
		Timestamp: resp.Timestamp,
		RequestID: requestID,
	})
}

func (h *MessageHandler) buildImageMessage(ctx context.Context, client *whatsmeow.Client, req SendMessageRequest) (*waE2E.Message, error) {
	if req.MediaURL == "" {
		return nil, fmt.Errorf("media_url is required for image messages")
	}

	// Download image
	data, err := h.downloadMedia(ctx, req.MediaURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	// Upload to WhatsApp
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

	// TODO: Convert to WebP if not already

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

func (h *MessageHandler) sendPresence(client *whatsmeow.Client, req SendMessageRequest) error {
	if req.Presence == nil {
		return fmt.Errorf("presence data is required")
	}

	var presence types.Presence
	if req.Presence.State == "available" {
		presence = types.PresenceAvailable
	} else {
		presence = types.PresenceUnavailable
	}

	return client.SendPresence(presence)
}

func (h *MessageHandler) sendChatPresence(client *whatsmeow.Client, req SendMessageRequest) error {
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
	} else {
		state = types.ChatPresencePaused
	}

	return client.SendChatPresence(jid, state, types.ChatPresenceMediaText)
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

// Placeholder methods for other message operations
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented"})
}

func (h *MessageHandler) RevokeMessage(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented"})
}

func (h *MessageHandler) ReactToMessage(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented"})
}

func (h *MessageHandler) UpdateMessage(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented"})
}
