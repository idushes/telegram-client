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

// MessageSender содержит информацию об отправителе сообщения
type MessageSender struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	IsBot     bool   `json:"is_bot,omitempty"`
}

// MessageEntity представляет форматирование в тексте сообщения
type MessageEntity struct {
	Type     string `json:"type"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	URL      string `json:"url,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	Language string `json:"language,omitempty"`
}

// MessageInfo содержит информацию о сообщении
type MessageInfo struct {
	ID           int             `json:"id"`
	Date         int             `json:"date"`
	Text         string          `json:"text,omitempty"`
	Type         string          `json:"type"`
	IsOutgoing   bool            `json:"is_outgoing,omitempty"`
	IsMentioned  bool            `json:"is_mentioned,omitempty"`
	MediaType    string          `json:"media_type,omitempty"`
	Sender       *MessageSender  `json:"sender,omitempty"`
	ForwardFrom  *MessageSender  `json:"forward_from,omitempty"`
	ReplyToMsgID int             `json:"reply_to_msg_id,omitempty"`
	Entities     []MessageEntity `json:"entities,omitempty"`
	Views        int             `json:"views,omitempty"`
	EditDate     int             `json:"edit_date,omitempty"`
}

// MessagesResponse содержит список сообщений для вывода в JSON
type MessagesResponse struct {
	Messages []MessageInfo `json:"messages"`
	Count    int           `json:"count"`
	ChatID   int64         `json:"chat_id"`
}

// GetMessages получает сообщения из указанного чата
func GetMessages(ctx context.Context, config AuthConfig, chatID int64, limit int) error {
	// Create client
	client := telegram.NewClient(config.AppID, config.AppHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: config.SessionFile,
		},
	})

	// Канал для передачи результата или ошибки
	resultCh := make(chan *MessagesResponse, 1)
	errCh := make(chan error, 1)

	// Запускаем клиент
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
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

			// Создаем InputPeer из chatID
			peer, err := getInputPeerFromChatID(ctx, client, chatID)
			if err != nil {
				return fmt.Errorf("failed to get input peer: %w", err)
			}

			fmt.Printf("Getting messages from chat ID %d...\n", chatID)

			// Получаем сообщения из чата
			history, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:  peer,
				Limit: limit,
			})
			if err != nil {
				return fmt.Errorf("failed to get messages: %w", err)
			}

			// Преобразуем полученные данные
			messages, err := extractMessages(history, chatID)
			if err != nil {
				return fmt.Errorf("failed to extract messages: %w", err)
			}

			// Отправляем результат
			resultCh <- messages
			return nil
		})

		if err != nil {
			errCh <- err
		}
	}()

	// Ждем или результат, или ошибку, или таймаут
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case result := <-resultCh:
		// Выводим результат в формате JSON
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to convert to JSON: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	case <-time.After(2 * time.Minute): // Таймаут 2 минуты
		return fmt.Errorf("operation timed out")
	}
}

// getInputPeerFromChatID преобразует ID чата в InputPeer
func getInputPeerFromChatID(ctx context.Context, client *telegram.Client, chatID int64) (tg.InputPeerClass, error) {
	fmt.Printf("Looking for peer with ID: %d\n", chatID)

	// Перед запуском API, попробуем определить, какой тип чата это может быть
	// и сконвертировать ID в правильный формат для поиска
	var rawID int64
	isChannel := false

	// Если ID отрицательный
	if chatID < 0 {
		// Проверяем, это канал/супергруппа (-100...)
		if chatID <= -1000000000000 {
			// Извлекаем ID канала из ID с префиксом
			rawID = -(chatID + 1000000000000)
			isChannel = true
			fmt.Printf("Detected channel/supergroup, using internal ID: %d\n", rawID)
		} else {
			// Обычный чат
			rawID = -chatID
			fmt.Printf("Detected regular chat, using internal ID: %d\n", rawID)
		}
	} else {
		// Если ID положительный (пользователь)
		rawID = chatID
		fmt.Printf("Detected user, using ID: %d\n", rawID)
	}

	// Получаем список диалогов для поиска информации о чатах и пользователях
	fmt.Println("Looking for peer in dialogs...")

	// Получаем список диалогов
	dialogsClass, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100, // Ограничиваем количество диалогов для скорости
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dialogs: %w", err)
	}

	// Ищем наш чат/пользователя среди диалогов
	var chats []tg.ChatClass
	var users []tg.UserClass
	switch d := dialogsClass.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
		users = d.Users
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
		users = d.Users
	default:
		return nil, fmt.Errorf("unexpected type of dialogs: %T", dialogsClass)
	}

	// Выводим информацию о найденных чатах для отладки
	fmt.Printf("Found %d chats and %d users\n", len(chats), len(users))

	// Если ID положительный, это пользователь
	if chatID > 0 {
		// Ищем пользователя по ID
		for _, user := range users {
			if u, ok := user.(*tg.User); ok {
				fmt.Printf("Checking user ID: %d\n", u.ID)
				if u.ID == rawID {
					fmt.Printf("Found user with ID %d, access hash: %d\n", u.ID, u.AccessHash)
					return &tg.InputPeerUser{
						UserID:     u.ID,
						AccessHash: u.AccessHash,
					}, nil
				}
			}
		}

		// Если это ваш собственный аккаунт, можно использовать InputPeerSelf
		// Но сначала проверим, что мы ищем именно ваш ID
		self, err := client.Self(ctx)
		if err == nil && self.ID == rawID {
			fmt.Println("Using InputPeerSelf for your own account")
			return &tg.InputPeerSelf{}, nil
		}

		return nil, fmt.Errorf("user with ID %d not found in your dialogs", chatID)
	}

	// Если это канал/супергруппа
	if isChannel {
		// Ищем канал по ID
		for _, chat := range chats {
			if c, ok := chat.(*tg.Channel); ok {
				fmt.Printf("Checking channel ID: %d\n", c.ID)
				if c.ID == rawID {
					fmt.Printf("Found channel with ID %d, access hash: %d\n", c.ID, c.AccessHash)
					return &tg.InputPeerChannel{
						ChannelID:  c.ID,
						AccessHash: c.AccessHash,
					}, nil
				}
			}
		}
	} else {
		// Это обычный групповой чат
		for _, chat := range chats {
			if c, ok := chat.(*tg.Chat); ok {
				fmt.Printf("Checking chat ID: %d\n", c.ID)
				if c.ID == rawID {
					fmt.Printf("Found chat with ID %d\n", c.ID)
					return &tg.InputPeerChat{
						ChatID: c.ID,
					}, nil
				}
			}
		}
	}

	// Если не нашли, выводим более подробную информацию для отладки
	fmt.Println("Could not find peer with specified ID. Available chats:")
	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Chat:
			fmt.Printf("Chat: ID=%d, Title=%s\n", c.ID, c.Title)
		case *tg.Channel:
			cType := "Channel"
			if c.Megagroup {
				cType = "Supergroup"
			}
			fmt.Printf("%s: ID=%d, Title=%s, Username=%s\n", cType, c.ID, c.Title, c.Username)
		}
	}

	return nil, fmt.Errorf("chat with ID %d not found in your dialogs", chatID)
}

// extractMessages извлекает информацию о сообщениях из ответа API
func extractMessages(historyClass tg.MessagesMessagesClass, chatID int64) (*MessagesResponse, error) {
	var messages []tg.MessageClass
	var users []tg.UserClass
	var chats []tg.ChatClass

	// Извлекаем данные в зависимости от типа полученного ответа
	switch h := historyClass.(type) {
	case *tg.MessagesMessages:
		messages = h.Messages
		users = h.Users
		chats = h.Chats
	case *tg.MessagesMessagesSlice:
		messages = h.Messages
		users = h.Users
		chats = h.Chats
	case *tg.MessagesChannelMessages:
		messages = h.Messages
		users = h.Users
		chats = h.Chats
	default:
		return nil, fmt.Errorf("unexpected type of messages: %T", historyClass)
	}

	// Создаем карты для быстрого доступа к пользователям и чатам по ID
	userMap := make(map[int64]tg.UserClass)
	chatMap := make(map[int64]tg.ChatClass)

	// Заполняем карту пользователей
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.GetID()] = user
		}
	}

	// Заполняем карту чатов
	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Chat:
			chatMap[chat.GetID()] = chat
		case *tg.Channel:
			chatMap[chat.GetID()] = chat
		}
	}

	// Создаем результат
	result := &MessagesResponse{
		Messages: make([]MessageInfo, 0, len(messages)),
		ChatID:   chatID,
	}

	// Обрабатываем каждое сообщение
	for _, msgClass := range messages {
		// Преобразуем к сообщению
		msg, ok := msgClass.(*tg.Message)
		if !ok {
			// Пропускаем служебные сообщения и другие типы
			continue
		}

		// Базовая информация о сообщении
		msgInfo := MessageInfo{
			ID:          msg.ID,
			Date:        msg.Date,
			Text:        msg.Message,
			Type:        "message",
			IsOutgoing:  msg.Out,
			IsMentioned: msg.Mentioned,
		}

		// Информация об отправителе
		fromID, senderType := extractSenderInfo(msg)
		if fromID != 0 {
			// Ищем отправителя в зависимости от типа
			if senderType == "user" && userMap[fromID] != nil {
				user := userMap[fromID].(*tg.User)
				msgInfo.Sender = &MessageSender{
					ID:        user.GetID(),
					Type:      "user",
					Username:  user.Username,
					FirstName: user.FirstName,
					LastName:  user.LastName,
					IsBot:     user.Bot,
				}
			} else if (senderType == "chat" || senderType == "channel") && chatMap[fromID] != nil {
				var chatTitle, chatUsername string

				// Определяем тип чата
				if chat, ok := chatMap[fromID].(*tg.Chat); ok {
					chatTitle = chat.Title
				} else if channel, ok := chatMap[fromID].(*tg.Channel); ok {
					chatTitle = channel.Title
					chatUsername = channel.Username
				}

				msgInfo.Sender = &MessageSender{
					ID:       fromID,
					Type:     senderType,
					Username: chatUsername,
					// Для чатов используем title как FirstName
					FirstName: chatTitle,
				}
			}
		}

		// Если есть информация о форвардинге
		fwdFrom, ok := msg.GetFwdFrom()
		if ok {
			msgInfo.Type = "forwarded_message"
			// Информация о первоначальном отправителе
			if fwdFrom.FromID != nil {
				switch fromID := fwdFrom.FromID.(type) {
				case *tg.PeerUser:
					if user, ok := userMap[fromID.UserID]; ok {
						if u, ok := user.(*tg.User); ok {
							msgInfo.ForwardFrom = &MessageSender{
								ID:        u.GetID(),
								Type:      "user",
								Username:  u.Username,
								FirstName: u.FirstName,
								LastName:  u.LastName,
								IsBot:     u.Bot,
							}
						}
					}
				case *tg.PeerChannel:
					if channel, ok := chatMap[fromID.ChannelID]; ok {
						if c, ok := channel.(*tg.Channel); ok {
							channelType := "channel"
							if c.Megagroup {
								channelType = "supergroup"
							}
							msgInfo.ForwardFrom = &MessageSender{
								ID:        c.GetID(),
								Type:      channelType,
								Username:  c.Username,
								FirstName: c.Title,
							}
						}
					}
				}
			}
		}

		// Информация о replied сообщении
		replyTo, ok := msg.GetReplyTo()
		if ok {
			if replyHeader, ok := replyTo.(*tg.MessageReplyHeader); ok {
				msgInfo.ReplyToMsgID = replyHeader.ReplyToMsgID
			}
		}

		// Информация о форматировании текста
		entities, ok := msg.GetEntities()
		if ok && len(entities) > 0 {
			msgInfo.Entities = make([]MessageEntity, 0, len(entities))
			for _, entity := range entities {
				switch e := entity.(type) {
				case *tg.MessageEntityBold:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "bold",
						Offset: e.Offset,
						Length: e.Length,
					})
				case *tg.MessageEntityItalic:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "italic",
						Offset: e.Offset,
						Length: e.Length,
					})
				case *tg.MessageEntityCode:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "code",
						Offset: e.Offset,
						Length: e.Length,
					})
				case *tg.MessageEntityPre:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:     "pre",
						Offset:   e.Offset,
						Length:   e.Length,
						Language: e.Language,
					})
				case *tg.MessageEntityURL:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "url",
						Offset: e.Offset,
						Length: e.Length,
					})
				case *tg.MessageEntityTextURL:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "text_url",
						Offset: e.Offset,
						Length: e.Length,
						URL:    e.URL,
					})
				case *tg.MessageEntityMention:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "mention",
						Offset: e.Offset,
						Length: e.Length,
					})
				case *tg.MessageEntityMentionName:
					msgInfo.Entities = append(msgInfo.Entities, MessageEntity{
						Type:   "mention_name",
						Offset: e.Offset,
						Length: e.Length,
						UserID: e.UserID,
					})
				}
			}
		}

		// Информация о медиа
		media, ok := msg.GetMedia()
		if ok {
			msgInfo.Type = "media_message"
			switch media := media.(type) {
			case *tg.MessageMediaPhoto:
				msgInfo.MediaType = "photo"
			case *tg.MessageMediaDocument:
				if doc, ok := media.Document.(*tg.Document); ok {
					// Определяем тип документа по MIME-типу
					mimeType := doc.MimeType
					switch {
					case mimeType == "video/mp4":
						msgInfo.MediaType = "video"
					case mimeType == "audio/mpeg" || mimeType == "audio/mp3":
						msgInfo.MediaType = "audio"
					case mimeType == "image/gif":
						msgInfo.MediaType = "gif"
					case mimeType == "application/vnd.ms-excel" || mimeType == "application/msword" ||
						mimeType == "application/pdf" || mimeType == "application/zip":
						msgInfo.MediaType = "document"
					default:
						msgInfo.MediaType = "file"
					}
				}
			}
		}

		// Количество просмотров для сообщений в каналах
		views, ok := msg.GetViews()
		if ok {
			msgInfo.Views = int(views)
		}

		// Дата редактирования
		editDate, ok := msg.GetEditDate()
		if ok {
			msgInfo.EditDate = editDate
		}

		// Добавляем информацию в результат
		result.Messages = append(result.Messages, msgInfo)
	}

	result.Count = len(result.Messages)
	return result, nil
}

// extractSenderInfo извлекает информацию об отправителе сообщения
func extractSenderInfo(msg *tg.Message) (int64, string) {
	// Проверяем различные поля, где может содержаться отправитель
	fromID, ok := msg.GetFromID()
	if ok {
		switch peer := fromID.(type) {
		case *tg.PeerUser:
			return peer.UserID, "user"
		case *tg.PeerChat:
			return peer.ChatID, "chat"
		case *tg.PeerChannel:
			return peer.ChannelID, "channel"
		}
	}
	return 0, ""
}
