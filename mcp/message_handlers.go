package mcp

import (
	"context"
	"log"
	"time"

	"github.com/gotd/td/tg"
)

// handleTelegramUpdates обрабатывает обновления, полученные от Telegram
func (s *Server) handleTelegramUpdates(ctx context.Context, updates tg.UpdatesClass) {
	log.Println("Received Telegram updates")

	switch u := updates.(type) {
	case *tg.Updates:
		for _, update := range u.Updates {
			s.processUpdate(ctx, update)
		}
	case *tg.UpdatesCombined:
		for _, update := range u.Updates {
			s.processUpdate(ctx, update)
		}
	case *tg.UpdateShort:
		s.processUpdate(ctx, u.Update)
	}
}

// processUpdate обрабатывает одно обновление
func (s *Server) processUpdate(ctx context.Context, update tg.UpdateClass) {
	switch u := update.(type) {
	case *tg.UpdateNewMessage:
		s.handleNewMessage(u)
	case *tg.UpdateNewChannelMessage:
		s.handleNewChannelMessage(u)
	}
}

// handleNewMessage обрабатывает новое личное сообщение
func (s *Server) handleNewMessage(update *tg.UpdateNewMessage) {
	// Проверяем, что сообщение действительно является сообщением
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return // не сообщение, игнорируем
	}

	// Игнорируем свои сообщения
	if msg.Out {
		return
	}

	log.Printf("Received new message: %s", msg.Message)

	// Создаем параметры уведомления
	params := map[string]interface{}{
		"message_id":   msg.ID,
		"date":         msg.Date,
		"text":         msg.Message,
		"has_media":    msg.Media != nil,
		"received_at":  time.Now().Unix(),
		"message_type": "private",
	}

	// Добавляем информацию об отправителе
	if msg.FromID != nil {
		switch from := msg.FromID.(type) {
		case *tg.PeerUser:
			params["sender"] = map[string]interface{}{
				"id":   from.UserID,
				"type": "user",
			}
		}
	}

	// Отправляем уведомление
	s.SendNotification("telegram/new_message", params)
}

// handleNewChannelMessage обрабатывает новое сообщение в группе или канале
func (s *Server) handleNewChannelMessage(update *tg.UpdateNewChannelMessage) {
	// Проверяем, что сообщение действительно является сообщением
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return // не сообщение, игнорируем
	}

	// Игнорируем свои сообщения
	if msg.Out {
		return
	}

	log.Printf("Received new channel message: %s", msg.Message)

	// Создаем параметры уведомления
	params := map[string]interface{}{
		"message_id":   msg.ID,
		"date":         msg.Date,
		"text":         msg.Message,
		"has_media":    msg.Media != nil,
		"received_at":  time.Now().Unix(),
		"message_type": "channel",
	}

	// Добавляем информацию об отправителе
	if msg.FromID != nil {
		switch from := msg.FromID.(type) {
		case *tg.PeerUser:
			params["sender"] = map[string]interface{}{
				"id":   from.UserID,
				"type": "user",
			}
		}
	}

	// Добавляем информацию о чате
	if msg.PeerID != nil {
		switch peer := msg.PeerID.(type) {
		case *tg.PeerChannel:
			params["chat"] = map[string]interface{}{
				"id":   peer.ChannelID,
				"type": "channel",
			}
		case *tg.PeerChat:
			params["chat"] = map[string]interface{}{
				"id":   peer.ChatID,
				"type": "chat",
			}
		}
	}

	// Отправляем уведомление
	s.SendNotification("telegram/new_message", params)
}
