package wa

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
	"go.mau.fi/whatsmeow/types/events"
)

// SetupEventHandlers sets up all event handlers for a WhatsApp client
func SetupEventHandlers(mc *ManagedClient, webhookSender *webhooks.Sender) {
	mc.Client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleIncomingMessage(mc, webhookSender, v)

		case *events.Receipt:
			handleReceipt(mc, webhookSender, v)

		case *events.Connected:
			handleConnected(mc, webhookSender, v)

		case *events.Disconnected:
			handleDisconnected(mc, webhookSender)

		case *events.LoggedOut:
			handleLoggedOut(mc, webhookSender, v)

		case *events.HistorySync:
			handleHistorySync(mc, webhookSender, v)

		case *events.GroupInfo:
			handleGroupInfo(mc, webhookSender, v)

			// Optional events - uncomment if needed
			// case *events.Presence:
			// 	handlePresence(mc, webhookSender, v)
			// case *events.ChatPresence:
			// 	handleChatPresence(mc, webhookSender, v)
			// case *events.StreamReplaced:
			// 	handleStreamReplaced(mc, webhookSender, v)
			// case *events.TemporaryBan:
			// 	handleTemporaryBan(mc, webhookSender, v)
		}
	})
}

func handleIncomingMessage(mc *ManagedClient, ws *webhooks.Sender, evt *events.Message) {
	mc.mu.Lock()
	mc.LastActivity = time.Now()
	mc.mu.Unlock()

	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Str("from", evt.Info.Sender.String()).
		Str("message_id", evt.Info.ID).
		Bool("is_group", evt.Info.IsGroup).
		Msg("Received message")

	messageData := map[string]interface{}{
		"message_id": evt.Info.ID,
		"from":       evt.Info.Sender.String(),
		"timestamp":  evt.Info.Timestamp.Unix(),
		"is_group":   evt.Info.IsGroup,
		"from_me":    evt.Info.IsFromMe,
		"push_name":  evt.Info.PushName,
	}

	if evt.Info.IsGroup {
		messageData["chat_jid"] = evt.Info.Chat.String()
	}

	// Extract message content based on type
	if evt.Message.Conversation != nil && *evt.Message.Conversation != "" {
		messageData["type"] = "text"
		messageData["body"] = *evt.Message.Conversation
	} else if evt.Message.ExtendedTextMessage != nil {
		messageData["type"] = "text"
		messageData["body"] = *evt.Message.ExtendedTextMessage.Text
		if evt.Message.ExtendedTextMessage.ContextInfo != nil && evt.Message.ExtendedTextMessage.ContextInfo.StanzaID != nil {
			messageData["quoted_message_id"] = *evt.Message.ExtendedTextMessage.ContextInfo.StanzaID
		}
	} else if evt.Message.ImageMessage != nil {
		messageData["type"] = "image"
		if evt.Message.ImageMessage.Caption != nil {
			messageData["caption"] = *evt.Message.ImageMessage.Caption
		}
		if evt.Message.ImageMessage.Mimetype != nil {
			messageData["mime_type"] = *evt.Message.ImageMessage.Mimetype
		}
		if evt.Message.ImageMessage.URL != nil {
			messageData["url"] = *evt.Message.ImageMessage.URL
		}
	} else if evt.Message.VideoMessage != nil {
		messageData["type"] = "video"
		if evt.Message.VideoMessage.Caption != nil {
			messageData["caption"] = *evt.Message.VideoMessage.Caption
		}
		if evt.Message.VideoMessage.Mimetype != nil {
			messageData["mime_type"] = *evt.Message.VideoMessage.Mimetype
		}
		if evt.Message.VideoMessage.URL != nil {
			messageData["url"] = *evt.Message.VideoMessage.URL
		}
	} else if evt.Message.AudioMessage != nil {
		messageData["type"] = "audio"
		if evt.Message.AudioMessage.Mimetype != nil {
			messageData["mime_type"] = *evt.Message.AudioMessage.Mimetype
		}
		if evt.Message.AudioMessage.URL != nil {
			messageData["url"] = *evt.Message.AudioMessage.URL
		}
		if evt.Message.AudioMessage.PTT != nil {
			messageData["ptt"] = *evt.Message.AudioMessage.PTT
		}
	} else if evt.Message.DocumentMessage != nil {
		messageData["type"] = "document"
		if evt.Message.DocumentMessage.FileName != nil {
			messageData["file_name"] = *evt.Message.DocumentMessage.FileName
		}
		if evt.Message.DocumentMessage.Mimetype != nil {
			messageData["mime_type"] = *evt.Message.DocumentMessage.Mimetype
		}
		if evt.Message.DocumentMessage.URL != nil {
			messageData["url"] = *evt.Message.DocumentMessage.URL
		}
	} else if evt.Message.StickerMessage != nil {
		messageData["type"] = "sticker"
		if evt.Message.StickerMessage.Mimetype != nil {
			messageData["mime_type"] = *evt.Message.StickerMessage.Mimetype
		}
		if evt.Message.StickerMessage.URL != nil {
			messageData["url"] = *evt.Message.StickerMessage.URL
		}
	} else if evt.Message.LocationMessage != nil {
		messageData["type"] = "location"
		if evt.Message.LocationMessage.DegreesLatitude != nil {
			messageData["latitude"] = *evt.Message.LocationMessage.DegreesLatitude
		}
		if evt.Message.LocationMessage.DegreesLongitude != nil {
			messageData["longitude"] = *evt.Message.LocationMessage.DegreesLongitude
		}
		if evt.Message.LocationMessage.Name != nil {
			messageData["name"] = *evt.Message.LocationMessage.Name
		}
	} else if evt.Message.ContactMessage != nil {
		messageData["type"] = "contact"
		if evt.Message.ContactMessage.DisplayName != nil {
			messageData["display_name"] = *evt.Message.ContactMessage.DisplayName
		}
		if evt.Message.ContactMessage.Vcard != nil {
			messageData["vcard"] = *evt.Message.ContactMessage.Vcard
		}
	} else if evt.Message.PollCreationMessage != nil {
		messageData["type"] = "poll"
		if evt.Message.PollCreationMessage.Name != nil {
			messageData["question"] = *evt.Message.PollCreationMessage.Name
		}
		options := []string{}
		for _, opt := range evt.Message.PollCreationMessage.Options {
			if opt.OptionName != nil {
				options = append(options, *opt.OptionName)
			}
		}
		messageData["options"] = options
	} else if evt.Message.ReactionMessage != nil {
		messageData["type"] = "reaction"
		if evt.Message.ReactionMessage.Text != nil {
			messageData["emoji"] = *evt.Message.ReactionMessage.Text
		}
		if evt.Message.ReactionMessage.Key != nil && evt.Message.ReactionMessage.Key.ID != nil {
			messageData["reacted_message_id"] = *evt.Message.ReactionMessage.Key.ID
		}
	}

	// Send webhook
	if err := ws.SendInbound(mc.WaAccountID, "", messageData); err != nil {
		log.Error().
			Err(err).
			Str("wa_account_id", mc.WaAccountID).
			Msg("Failed to send inbound message webhook")
	}
}

func handleReceipt(mc *ManagedClient, ws *webhooks.Sender, evt *events.Receipt) {
	mc.mu.Lock()
	mc.LastActivity = time.Now()
	mc.mu.Unlock()

	log.Debug().
		Str("wa_account_id", mc.WaAccountID).
		Str("type", string(evt.Type)).
		Msg("Received receipt")

	for _, msgID := range evt.MessageIDs {
		if evt.Type == events.ReceiptTypeDelivered {
			if err := ws.SendDelivery(mc.WaAccountID, "", msgID); err != nil {
				log.Error().Err(err).Msg("Failed to send delivery webhook")
			}
		} else if evt.Type == events.ReceiptTypeRead {
			if err := ws.SendRead(mc.WaAccountID, "", msgID); err != nil {
				log.Error().Err(err).Msg("Failed to send read webhook")
			}
		}
	}
}

func handleConnected(mc *ManagedClient, ws *webhooks.Sender, evt *events.Connected) {
	mc.mu.Lock()
	mc.Connected = true
	mc.LastActivity = time.Now()
	mc.mu.Unlock()

	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Msg("WhatsApp connected")

	ws.SendStatus(mc.WaAccountID, "connected", "")
}

func handleDisconnected(mc *ManagedClient, ws *webhooks.Sender) {
	mc.mu.Lock()
	mc.Connected = false
	mc.mu.Unlock()

	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Msg("WhatsApp disconnected")

	ws.SendStatus(mc.WaAccountID, "disconnected", "")
}

func handleLoggedOut(mc *ManagedClient, ws *webhooks.Sender, evt *events.LoggedOut) {
	mc.mu.Lock()
	mc.Connected = false
	mc.mu.Unlock()

	reason := fmt.Sprintf("reason_%v", evt.Reason)

	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Str("reason", reason).
		Msg("WhatsApp logged out")

	ws.SendStatus(mc.WaAccountID, "logged_out", reason)
}

func handleHistorySync(mc *ManagedClient, ws *webhooks.Sender, evt *events.HistorySync) {
	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Int("conversation_count", len(evt.Data.Conversations)).
		Msg("History sync received")

	ws.Send("history_sync", webhooks.WebhookPayload{
		EventType:   "history_sync",
		WaAccountID: mc.WaAccountID,
		Data: map[string]interface{}{
			"type":               evt.Data.SyncType.String(),
			"conversation_count": len(evt.Data.Conversations),
		},
	})
}

func handleGroupInfo(mc *ManagedClient, ws *webhooks.Sender, evt *events.GroupInfo) {
	log.Info().
		Str("wa_account_id", mc.WaAccountID).
		Str("group_jid", evt.JID.String()).
		Msg("Group info update")

	data := map[string]interface{}{
		"group_jid": evt.JID.String(),
	}

	if evt.Name != nil {
		data["name"] = *evt.Name
	}
	if evt.Topic != nil {
		data["topic"] = *evt.Topic
	}

	ws.Send("group_info", webhooks.WebhookPayload{
		EventType:   "group_info",
		WaAccountID: mc.WaAccountID,
		Data:        data,
	})
}
