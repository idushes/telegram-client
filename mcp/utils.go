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
