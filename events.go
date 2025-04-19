package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// EventType представляет тип события
type EventType string

const (
	EventMessage    EventType = "message"     // Новое сообщение
	EventEdit       EventType = "edit"        // Редактирование сообщения
	EventDelete     EventType = "delete"      // Удаление сообщений
	EventRead       EventType = "read"        // Прочтение сообщений
	EventUserStatus EventType = "user_status" // Изменение статуса пользователя
	EventTyping     EventType = "typing"      // Печатает сообщение
	EventChatAction EventType = "chat_action" // Действие в чате (добавление/удаление участников)
)

// EventInfo содержит информацию о событии
type EventInfo struct {
	Type      EventType       `json:"type"`
	Time      int64           `json:"time"` // Unix timestamp
	ChatID    int64           `json:"chat_id,omitempty"`
	ChatType  string          `json:"chat_type,omitempty"`
	ChatTitle string          `json:"chat_title,omitempty"`
	UserID    int64           `json:"user_id,omitempty"`
	Username  string          `json:"username,omitempty"`
	FirstName string          `json:"first_name,omitempty"`
	LastName  string          `json:"last_name,omitempty"`
	MessageID int             `json:"message_id,omitempty"`
	Message   string          `json:"message,omitempty"`
	Action    string          `json:"action,omitempty"`
	RawData   json.RawMessage `json:"raw_data,omitempty"`
}

// GetEvents запускает отслеживание событий Telegram
func GetEvents(ctx context.Context, config AuthConfig, timeout int) error {
	// Создаем контекст с таймаутом, если указан
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// Создаем клиент
	client := telegram.NewClient(config.AppID, config.AppHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: config.SessionFile,
		},
	})

	// Запускаем клиент
	return client.Run(ctx, func(ctx context.Context) error {
		// Авторизируемся при необходимости
		flow := auth.NewFlow(
			auth.CodeOnly(config.Phone, &telegramCodeAuth{}),
			auth.SendCodeOptions{},
		)

		// Выполняем авторизацию если нужно
		fmt.Println("Checking authorization...")
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("authentication error: %w", err)
		}

		// Проверяем авторизацию
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth status: %w", err)
		}

		if !status.Authorized {
			return fmt.Errorf("not authorized")
		}

		fmt.Println("Starting events tracking...")

		// Получаем API клиент
		api := client.API()

		// Создаем обработчик обновлений
		dispatcher := tg.NewUpdateDispatcher()

		// Обработчик новых сообщений
		dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
			return handleNewMessage(entities, update)
		})

		// Обработчик редактирования сообщений
		dispatcher.OnEditMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateEditMessage) error {
			return handleEditMessage(entities, update)
		})

		// Создаем канал для получения обновлений
		updates := make(chan tg.UpdatesClass, 100)

		// Запускаем горутину для обработки обновлений
		go func() {
			for update := range updates {
				// Обрабатываем различные типы обновлений
				switch u := update.(type) {
				case *tg.Updates:
					for _, update := range u.Updates {
						handleUpdate(update)
					}
				case *tg.UpdatesCombined:
					for _, update := range u.Updates {
						handleUpdate(update)
					}
				case *tg.UpdateShort:
					handleUpdate(u.Update)
				}
			}
		}()

		// Получаем текущее состояние
		state, err := api.UpdatesGetState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get updates state: %w", err)
		}

		// Получаем разницу в обновлениях
		diff, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{
			Pts:  state.Pts,
			Date: state.Date,
			Qts:  state.Qts,
		})
		if err != nil {
			return fmt.Errorf("failed to get updates difference: %w", err)
		}

		// Обрабатываем полученные обновления
		switch d := diff.(type) {
		case *tg.UpdatesDifference:
			for _, update := range d.NewMessages {
				handleMessage(update)
			}
			for _, update := range d.OtherUpdates {
				handleUpdate(update)
			}
		case *tg.UpdatesDifferenceSlice:
			for _, update := range d.NewMessages {
				handleMessage(update)
			}
			for _, update := range d.OtherUpdates {
				handleUpdate(update)
			}
		}

		// Запускаем цикл получения обновлений
		for {
			select {
			case <-ctx.Done():
				close(updates)
				return ctx.Err()
			default:
				// Получаем обновления
				updateResp, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{
					Pts:  state.Pts,
					Date: state.Date,
					Qts:  state.Qts,
				})
				if err != nil {
					fmt.Printf("Error getting updates: %v\n", err)
					time.Sleep(5 * time.Second)
					continue
				}

				// Обрабатываем полученные обновления
				switch d := updateResp.(type) {
				case *tg.UpdatesDifference:
					for _, update := range d.NewMessages {
						handleMessage(update)
					}
					for _, update := range d.OtherUpdates {
						handleUpdate(update)
					}

					// Обновляем состояние
					state.Pts = d.State.Pts
					state.Date = d.State.Date
					state.Qts = d.State.Qts

				case *tg.UpdatesDifferenceSlice:
					for _, update := range d.NewMessages {
						handleMessage(update)
					}
					for _, update := range d.OtherUpdates {
						handleUpdate(update)
					}

					// Обновляем состояние
					state.Pts = d.IntermediateState.Pts
					state.Date = d.IntermediateState.Date
					state.Qts = d.IntermediateState.Qts

				case *tg.UpdatesDifferenceEmpty:
					// Нет новых обновлений
					state.Date = d.Date
				}

				// Небольшая пауза перед следующим запросом
				time.Sleep(1 * time.Second)
			}
		}
	})
}

// handleMessage обрабатывает сообщение
func handleMessage(message tg.MessageClass) {
	msg, ok := message.(*tg.Message)
	if !ok {
		return
	}

	// Создаем информацию о событии
	event := EventInfo{
		Type:      EventMessage,
		Time:      time.Now().Unix(),
		MessageID: msg.ID,
		Message:   msg.Message,
	}

	// Определяем чат
	if msg.PeerID != nil {
		switch peer := msg.PeerID.(type) {
		case *tg.PeerUser:
			event.ChatID = peer.UserID
			event.ChatType = "user"
		case *tg.PeerChat:
			event.ChatID = -peer.ChatID
			event.ChatType = "chat"
		case *tg.PeerChannel:
			event.ChatID = -1000000000000 - peer.ChannelID
			event.ChatType = "channel"
		}
	}

	// Определяем отправителя
	if msg.FromID != nil {
		if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
			event.UserID = fromUser.UserID
		}
	}

	// Выводим событие в формате JSON
	outputEvent(event)
}

// handleUpdate обрабатывает обновление
func handleUpdate(update tg.UpdateClass) {
	switch u := update.(type) {
	case *tg.UpdateNewMessage:
		if msg, ok := u.Message.(*tg.Message); ok {
			// Создаем информацию о событии
			event := EventInfo{
				Type:      EventMessage,
				Time:      time.Now().Unix(),
				MessageID: msg.ID,
				Message:   msg.Message,
			}

			// Определяем чат
			if msg.PeerID != nil {
				switch peer := msg.PeerID.(type) {
				case *tg.PeerUser:
					event.ChatID = peer.UserID
					event.ChatType = "user"
				case *tg.PeerChat:
					event.ChatID = -peer.ChatID
					event.ChatType = "chat"
				case *tg.PeerChannel:
					event.ChatID = -1000000000000 - peer.ChannelID
					event.ChatType = "channel"
				}
			}

			// Определяем отправителя
			if msg.FromID != nil {
				if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
					event.UserID = fromUser.UserID
				}
			}

			// Выводим событие в формате JSON
			outputEvent(event)
		}

	case *tg.UpdateEditMessage:
		if msg, ok := u.Message.(*tg.Message); ok {
			// Создаем информацию о событии
			event := EventInfo{
				Type:      EventEdit,
				Time:      time.Now().Unix(),
				MessageID: msg.ID,
				Message:   msg.Message,
			}

			// Определяем чат
			if msg.PeerID != nil {
				switch peer := msg.PeerID.(type) {
				case *tg.PeerUser:
					event.ChatID = peer.UserID
					event.ChatType = "user"
				case *tg.PeerChat:
					event.ChatID = -peer.ChatID
					event.ChatType = "chat"
				case *tg.PeerChannel:
					event.ChatID = -1000000000000 - peer.ChannelID
					event.ChatType = "channel"
				}
			}

			// Определяем отправителя
			if msg.FromID != nil {
				if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
					event.UserID = fromUser.UserID
				}
			}

			// Выводим событие в формате JSON
			outputEvent(event)
		}

	case *tg.UpdateDeleteMessages:
		// Создаем информацию о событии
		event := EventInfo{
			Type: EventDelete,
			Time: time.Now().Unix(),
		}

		// Сериализуем ID удаленных сообщений
		messageIDs, _ := json.Marshal(u.Messages)
		event.RawData = messageIDs

		// Выводим событие в формате JSON
		outputEvent(event)

	case *tg.UpdateUserStatus:
		// Создаем информацию о событии
		event := EventInfo{
			Type:   EventUserStatus,
			Time:   time.Now().Unix(),
			UserID: u.UserID,
		}

		// Определяем статус
		switch status := u.Status.(type) {
		case *tg.UserStatusOnline:
			event.Action = "online"
		case *tg.UserStatusOffline:
			event.Action = "offline"
		case *tg.UserStatusRecently:
			event.Action = "recently"
		case *tg.UserStatusLastWeek:
			event.Action = "last_week"
		case *tg.UserStatusLastMonth:
			event.Action = "last_month"
		default:
			event.Action = fmt.Sprintf("unknown_status_%T", status)
		}

		// Выводим событие в формате JSON
		outputEvent(event)

	case *tg.UpdateUserTyping:
		// Создаем информацию о событии
		event := EventInfo{
			Type:   EventTyping,
			Time:   time.Now().Unix(),
			UserID: u.UserID,
		}

		// Определяем действие
		switch action := u.Action.(type) {
		case *tg.SendMessageTypingAction:
			event.Action = "typing"
		case *tg.SendMessageRecordVideoAction:
			event.Action = "recording_video"
		case *tg.SendMessageUploadVideoAction:
			event.Action = "uploading_video"
		case *tg.SendMessageRecordAudioAction:
			event.Action = "recording_audio"
		case *tg.SendMessageUploadAudioAction:
			event.Action = "uploading_audio"
		case *tg.SendMessageUploadPhotoAction:
			event.Action = "uploading_photo"
		case *tg.SendMessageUploadDocumentAction:
			event.Action = "uploading_document"
		case *tg.SendMessageGeoLocationAction:
			event.Action = "choosing_location"
		case *tg.SendMessageChooseContactAction:
			event.Action = "choosing_contact"
		default:
			event.Action = fmt.Sprintf("unknown_action_%T", action)
		}

		// Выводим событие в формате JSON
		outputEvent(event)
	}
}

// Обработчик новых сообщений
func handleNewMessage(entities tg.Entities, update *tg.UpdateNewMessage) error {
	// Получаем сообщение
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return nil // Пропускаем, если это не сообщение
	}

	// Создаем информацию о событии
	event := EventInfo{
		Type:      EventMessage,
		Time:      time.Now().Unix(),
		MessageID: msg.ID,
		Message:   msg.Message,
	}

	// Определяем чат
	switch peer := msg.PeerID.(type) {
	case *tg.PeerUser:
		event.ChatID = peer.UserID
		event.ChatType = "user"
		// Имя пользователя можно получить из entities, если доступно
	case *tg.PeerChat:
		event.ChatID = -peer.ChatID // Используем отрицательный ID для групповых чатов
		event.ChatType = "chat"
	case *tg.PeerChannel:
		event.ChatID = -1000000000000 - peer.ChannelID // Используем формат -100XXXXXXXXXX для каналов
		event.ChatType = "channel"
	}

	// Определяем отправителя
	if msg.FromID != nil {
		if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
			event.UserID = fromUser.UserID
			// Имя отправителя можно получить из entities, если доступно
		}
	}

	// Выводим событие в формате JSON
	return outputEvent(event)
}

// Обработчик редактирования сообщений
func handleEditMessage(entities tg.Entities, update *tg.UpdateEditMessage) error {
	// Получаем сообщение
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return nil // Пропускаем, если это не сообщение
	}

	// Создаем информацию о событии
	event := EventInfo{
		Type:      EventEdit,
		Time:      time.Now().Unix(),
		MessageID: msg.ID,
		Message:   msg.Message,
	}

	// Определяем чат
	switch peer := msg.PeerID.(type) {
	case *tg.PeerUser:
		event.ChatID = peer.UserID
		event.ChatType = "user"
	case *tg.PeerChat:
		event.ChatID = -peer.ChatID
		event.ChatType = "chat"
	case *tg.PeerChannel:
		event.ChatID = -1000000000000 - peer.ChannelID
		event.ChatType = "channel"
	}

	// Определяем отправителя
	if msg.FromID != nil {
		if fromUser, ok := msg.FromID.(*tg.PeerUser); ok {
			event.UserID = fromUser.UserID
		}
	}

	// Выводим событие в формате JSON
	return outputEvent(event)
}

// outputEvent выводит событие в формате JSON
func outputEvent(event EventInfo) error {
	// Сериализуем структуру в JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event to JSON: %w", err)
	}

	// Выводим в консоль
	fmt.Println(string(jsonData))
	return nil
}
