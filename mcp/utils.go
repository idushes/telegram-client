package mcp

import (
	"strings"

	"github.com/gotd/td/tg"
)

// isConnectionError проверяет, является ли ошибка связанной с соединением
func isConnectionError(err error) bool {
	errStr := err.Error()
	connectionErrors := []string{
		"connection",
		"dead",
		"timeout",
		"closed",
		"broken pipe",
		"reset by peer",
		"EOF",
		"i/o timeout",
	}

	for _, e := range connectionErrors {
		if strings.Contains(errStr, e) {
			return true
		}
	}

	return false
}

// max возвращает наибольшее из двух целых чисел
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractGroupInfo извлекает информацию о группе из чата
func extractGroupInfo(chat tg.ChatClass) map[string]interface{} {
	switch c := chat.(type) {
	case *tg.Chat:
		return map[string]interface{}{
			"id":          c.ID,
			"title":       c.Title,
			"type":        "chat",
			"members":     c.ParticipantsCount,
			"deactivated": c.Deactivated,
		}
	case *tg.ChatForbidden:
		return map[string]interface{}{
			"id":    c.ID,
			"title": c.Title,
			"type":  "chat_forbidden",
		}
	case *tg.Channel:
		// Фильтруем только группы, а не каналы
		if c.Megagroup {
			return map[string]interface{}{
				"id":         c.ID,
				"title":      c.Title,
				"type":       "megagroup",
				"username":   c.Username,
				"members":    c.ParticipantsCount,
				"verified":   c.Verified,
				"restricted": c.Restricted,
			}
		}
		return nil
	case *tg.ChannelForbidden:
		// Фильтруем только группы, а не каналы
		if c.Megagroup {
			return map[string]interface{}{
				"id":    c.ID,
				"title": c.Title,
				"type":  "megagroup_forbidden",
			}
		}
		return nil
	default:
		return nil
	}
}

// extractMessageInfo извлекает информацию из сообщения
func extractMessageInfo(message tg.MessageClass) map[string]interface{} {
	switch m := message.(type) {
	case *tg.Message:
		result := map[string]interface{}{
			"id":        m.ID,
			"date":      m.Date,
			"out":       m.Out,
			"mentioned": m.Mentioned,
			"media":     m.Media != nil,
		}

		// Добавляем текст, если он есть
		if m.Message != "" {
			result["text"] = m.Message
		}

		// Добавляем информацию об отправителе
		if m.FromID != nil {
			result["from"] = extractPeerInfo(m.FromID)
		}

		// Если есть медиа, добавляем информацию о нем
		if m.Media != nil {
			result["media_type"] = getMediaType(m.Media)
		}

		return result
	case *tg.MessageService:
		return map[string]interface{}{
			"id":     m.ID,
			"date":   m.Date,
			"type":   "service_message",
			"action": getServiceActionType(m.Action),
		}
	default:
		return nil
	}
}

// extractPeerInfo извлекает информацию о пире (пользователе, группе, канале)
func extractPeerInfo(peer tg.PeerClass) map[string]interface{} {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return map[string]interface{}{
			"type": "user",
			"id":   p.UserID,
		}
	case *tg.PeerChat:
		return map[string]interface{}{
			"type": "chat",
			"id":   p.ChatID,
		}
	case *tg.PeerChannel:
		return map[string]interface{}{
			"type": "channel",
			"id":   p.ChannelID,
		}
	default:
		return map[string]interface{}{
			"type": "unknown",
		}
	}
}

// getMediaType возвращает строковое представление типа медиа
func getMediaType(media tg.MessageMediaClass) string {
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaGeo:
		return "geo"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaPoll:
		return "poll"
	case *tg.MessageMediaWebPage:
		return "webpage"
	default:
		return "unknown"
	}
}

// getServiceActionType возвращает строковое представление типа сервисного действия
func getServiceActionType(action tg.MessageActionClass) string {
	switch action.(type) {
	case *tg.MessageActionChatCreate:
		return "chat_created"
	case *tg.MessageActionChatEditTitle:
		return "chat_title_edited"
	case *tg.MessageActionChatEditPhoto:
		return "chat_photo_edited"
	case *tg.MessageActionChatDeletePhoto:
		return "chat_photo_deleted"
	case *tg.MessageActionChatAddUser:
		return "user_added"
	case *tg.MessageActionChatDeleteUser:
		return "user_removed"
	case *tg.MessageActionChannelCreate:
		return "channel_created"
	case *tg.MessageActionPinMessage:
		return "message_pinned"
	default:
		return "other"
	}
}
