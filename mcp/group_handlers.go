package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetGroups обрабатывает запрос на получение списка групп в Telegram
func (s *Server) handleGetGroups(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Проверяем, что клиент инициализирован
	if s.Client == nil {
		// Пробуем восстановить клиент
		go s.checkClientStatus(context.Background())
		return mcp.NewToolResultErrorFromErr("Telegram client not initialized, trying to reconnect. Please try again in a few seconds.", errors.New("client not initialized")), nil
	}

	// Проверяем готовность клиента
	if !s.IsClientReady() {
		log.Println("Telegram client is not ready")
		// Пробуем восстановить клиент, если он не готов
		go s.checkClientStatus(context.Background())
		return mcp.NewToolResultErrorFromErr("Telegram client is not ready. System is reconnecting, please try again in a few seconds.", errors.New("client not ready")), nil
	}

	// Получаем параметр limit (по умолчанию 50)
	userLimit := 50
	if limitParam, ok := req.Params.Arguments["limit"]; ok {
		if limitValue, ok := limitParam.(float64); ok {
			userLimit = int(limitValue)
		}
	}

	// Telegram API возвращает примерно в 2 раза больше элементов, чем запрошено
	// Корректируем запрашиваемый лимит, чтобы получить примерно то количество, которое запросил пользователь
	apiLimit := userLimit
	if userLimit > 0 {
		// Если пользователь указал конкретный лимит, делим его на 2 (но не меньше 1)
		apiLimit = max(1, userLimit/2)
	}

	log.Printf("Getting list of Telegram groups with user limit %d (API limit: %d)", userLimit, apiLimit)

	// Проверяем авторизацию клиента
	if err := s.checkClientAuth(ctx); err != nil {
		return err, nil
	}

	// Получаем группы от API
	return s.getGroupsFromAPI(ctx, userLimit, apiLimit)
}

// checkClientAuth проверяет авторизацию клиента
func (s *Server) checkClientAuth(ctx context.Context) *mcp.CallToolResult {
	// Создаем контекст с таймаутом для запроса
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Проверяем, авторизован ли клиент
	// Создаем канал для получения статуса авторизации
	authStatus := make(chan bool, 1)
	authErrCh := make(chan error, 1)

	// Запускаем горутину для проверки авторизации
	go func() {
		// Проверяем, авторизован ли клиент
		status, err := s.Client.Auth().Status(reqCtx)
		if err != nil {
			authErrCh <- err
			return
		}
		authStatus <- status.Authorized
	}()

	// Ждем результата проверки авторизации с таймаутом
	select {
	case err := <-authErrCh:
		log.Printf("Error checking auth status: %v", err)

		// Устанавливаем флаг неготовности клиента
		s.ClientMutex.Lock()
		s.ClientReady = false
		s.ClientMutex.Unlock()

		// Проверяем на критические ошибки
		if isFatalClientError(err) {
			// Для критических ошибок запускаем восстановление клиента
			log.Printf("Fatal client error detected: %v, recreating client...", err)
			go func() {
				// Небольшая задержка перед пересозданием
				time.Sleep(1 * time.Second)
				s.checkClientStatus(context.Background())
			}()
			return mcp.NewToolResultErrorFromErr("Telegram connection error detected. System is reconnecting, please try again in a few seconds.", err)
		}

		// Запускаем проверку состояния клиента
		go s.checkClientStatus(context.Background())

		return mcp.NewToolResultErrorFromErr("Failed to check auth status, please try again later", err)
	case authorized := <-authStatus:
		if !authorized {
			log.Printf("User not authorized")
			return mcp.NewToolResultErrorFromErr("User not authorized", errors.New("authentication required"))
		}
	case <-reqCtx.Done():
		log.Printf("Auth status check timed out")

		// Устанавливаем флаг неготовности клиента
		s.ClientMutex.Lock()
		s.ClientReady = false
		s.ClientMutex.Unlock()

		// Запускаем проверку состояния клиента
		go s.checkClientStatus(context.Background())

		return mcp.NewToolResultErrorFromErr("Auth status check timed out, please try again later", reqCtx.Err())
	}

	return nil
}

// getGroupsFromAPI получает группы от Telegram API
func (s *Server) getGroupsFromAPI(ctx context.Context, userLimit, apiLimit int) (*mcp.CallToolResult, error) {
	// Создаем контекст с таймаутом для запроса
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Создаем API клиент
	api := s.Client.API()

	// Максимальное количество попыток
	const maxRetries = 3
	var lastErr error

	// Попытки получения диалогов с повторами при ошибке соединения
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry attempt %d/%d", attempt, maxRetries)
			time.Sleep(2 * time.Second) // Пауза между попытками
		}

		// Создаем запрос на получение диалогов
		request := &tg.MessagesGetDialogsRequest{
			Limit:      apiLimit, // Используем скорректированный лимит для API
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
			Hash:       0,
		}

		// Выполняем запрос
		dialogs, err := api.MessagesGetDialogs(reqCtx, request)
		if err != nil {
			lastErr = err
			log.Printf("Error getting dialogs (attempt %d/%d): %v", attempt+1, maxRetries, err)

			// Проверяем тип ошибки
			if strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "dead") ||
				strings.Contains(err.Error(), "timeout") {
				// Это ошибка соединения, будем повторять
				continue
			}

			// Другой тип ошибки, не связанный с соединением
			return mcp.NewToolResultErrorFromErr("Failed to get Telegram groups", err), nil
		}

		// Подготавливаем результат
		groups := []map[string]interface{}{}

		// Обрабатываем результат в зависимости от типа
		switch d := dialogs.(type) {
		case *tg.MessagesDialogs:
			// Фильтруем только группы из списка диалогов
			for _, chat := range d.Chats {
				group := extractGroupInfo(chat)
				if group != nil {
					groups = append(groups, group)
					// Проверяем, не превысили ли мы запрошенный пользователем лимит
					if userLimit > 0 && len(groups) >= userLimit {
						break
					}
				}
			}
		case *tg.MessagesDialogsSlice:
			// Фильтруем только группы из списка диалогов
			for _, chat := range d.Chats {
				group := extractGroupInfo(chat)
				if group != nil {
					groups = append(groups, group)
					// Проверяем, не превысили ли мы запрошенный пользователем лимит
					if userLimit > 0 && len(groups) >= userLimit {
						break
					}
				}
			}
		default:
			log.Printf("Unknown dialogs type: %T", dialogs)
			return mcp.NewToolResultErrorFromErr("Unknown dialogs response type", errors.New("unexpected response type")), nil
		}

		log.Printf("Found %d groups (requested limit: %d)", len(groups), userLimit)

		// Сериализуем результат в JSON
		resultObj := map[string]interface{}{
			"groups": groups,
			"count":  len(groups),
		}

		resultText, err := json.Marshal(resultObj)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to serialize result", err), nil
		}

		// Возвращаем результат
		return mcp.NewToolResultText(string(resultText)), nil
	}

	// Если мы здесь, значит все попытки не удались
	return mcp.NewToolResultErrorFromErr(
		fmt.Sprintf("Failed to get Telegram groups after %d attempts", maxRetries),
		lastErr), nil
}

// handleGetGroupMessages обрабатывает запрос на получение сообщений из группы
func (s *Server) handleGetGroupMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Проверяем, что клиент инициализирован
	if s.Client == nil {
		// Пробуем восстановить клиент
		go s.checkClientStatus(context.Background())
		return mcp.NewToolResultErrorFromErr("Telegram client not initialized, trying to reconnect. Please try again in a few seconds.", errors.New("client not initialized")), nil
	}

	// Проверяем готовность клиента
	if !s.IsClientReady() {
		log.Println("Telegram client is not ready")
		// Пробуем восстановить клиент, если он не готов
		go s.checkClientStatus(context.Background())
		return mcp.NewToolResultErrorFromErr("Telegram client is not ready. System is reconnecting, please try again in a few seconds.", errors.New("client not ready")), nil
	}

	// Получаем ID группы из параметров
	var groupID int64
	if groupIDParam, ok := req.Params.Arguments["group_id"]; ok {
		if groupIDValue, ok := groupIDParam.(float64); ok {
			groupID = int64(groupIDValue)
		} else {
			return mcp.NewToolResultErrorFromErr("Invalid group_id format", errors.New("group_id must be a number")), nil
		}
	} else {
		return mcp.NewToolResultErrorFromErr("Missing group_id parameter", errors.New("group_id parameter required")), nil
	}

	// Получаем параметр limit (по умолчанию 20)
	limit := 20
	if limitParam, ok := req.Params.Arguments["limit"]; ok {
		if limitValue, ok := limitParam.(float64); ok {
			limit = int(limitValue)
		}
	}

	log.Printf("Getting messages from group ID %d with limit %d", groupID, limit)

	// Проверяем авторизацию клиента
	if err := s.checkClientAuth(ctx); err != nil {
		return err, nil
	}

	// Получаем сообщения из группы
	return s.getMessagesFromGroup(ctx, groupID, limit)
}

// getMessagesFromGroup получает сообщения из группы по ID
func (s *Server) getMessagesFromGroup(ctx context.Context, groupID int64, limit int) (*mcp.CallToolResult, error) {
	// Создаем контекст с таймаутом для запроса
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Создаем API клиент
	api := s.Client.API()

	// Создаем InputPeer для обращения к группе
	peer, err := s.getInputPeerForGroup(ctx, groupID)
	if err != nil {
		log.Printf("Error creating input peer: %v", err)
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Failed to create InputPeer for group %d: %v", groupID, err), err), nil
	}

	log.Printf("Successfully created InputPeer for group %d: %T", groupID, peer)

	// Максимальное количество попыток
	const maxRetries = 3
	var lastErr error

	// Попытки получения сообщений с повторами при ошибке соединения
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry attempt %d/%d", attempt, maxRetries)
			time.Sleep(2 * time.Second) // Пауза между попытками
		}

		// Создаем запрос на получение истории сообщений
		request := &tg.MessagesGetHistoryRequest{
			Peer:       peer,
			OffsetID:   0,
			OffsetDate: 0,
			AddOffset:  0,
			Limit:      limit,
			MaxID:      0,
			MinID:      0,
			Hash:       0,
		}

		log.Printf("Sending GetHistory request for peer: %T", peer)

		// Выполняем запрос
		history, err := api.MessagesGetHistory(reqCtx, request)
		if err != nil {
			lastErr = err
			log.Printf("Error getting messages (attempt %d/%d): %v", attempt+1, maxRetries, err)

			// Если ошибка связана с FLOOD_WAIT, делаем более длительную задержку
			if strings.Contains(err.Error(), "FLOOD_WAIT") {
				waitSec := 5 // По умолчанию ждем 5 секунд
				// Пытаемся извлечь число секунд из сообщения об ошибке
				parts := strings.Split(err.Error(), "_")
				if len(parts) > 0 {
					lastPart := parts[len(parts)-1]
					if waitTime, parseErr := strconv.Atoi(lastPart); parseErr == nil {
						waitSec = waitTime
					}
				}
				log.Printf("Flood wait detected, waiting for %d seconds", waitSec)
				time.Sleep(time.Duration(waitSec) * time.Second)
				continue
			}

			// Проверяем тип ошибки
			if strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "dead") ||
				strings.Contains(err.Error(), "timeout") {
				// Это ошибка соединения, будем повторять
				continue
			}

			// Другой тип ошибки, не связанный с соединением
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Failed to get messages from group %d: %v", groupID, err), err), nil
		}

		// Подготавливаем результат
		messages := []map[string]interface{}{}

		// Обрабатываем результат в зависимости от типа
		switch h := history.(type) {
		case *tg.MessagesChannelMessages:
			log.Printf("Received ChannelMessages with %d messages", len(h.Messages))
			// Преобразуем сообщения в удобный формат
			for _, msg := range h.Messages {
				msgMap := extractMessageInfo(msg)
				if msgMap != nil {
					messages = append(messages, msgMap)
				}
			}
		case *tg.MessagesMessages:
			log.Printf("Received Messages with %d messages", len(h.Messages))
			// Преобразуем сообщения в удобный формат
			for _, msg := range h.Messages {
				msgMap := extractMessageInfo(msg)
				if msgMap != nil {
					messages = append(messages, msgMap)
				}
			}
		case *tg.MessagesMessagesSlice:
			log.Printf("Received MessagesSlice with %d messages", len(h.Messages))
			// Преобразуем сообщения в удобный формат
			for _, msg := range h.Messages {
				msgMap := extractMessageInfo(msg)
				if msgMap != nil {
					messages = append(messages, msgMap)
				}
			}
		default:
			log.Printf("Unknown history type: %T", history)
			return mcp.NewToolResultErrorFromErr("Unknown history response type", errors.New("unexpected response type")), nil
		}

		log.Printf("Found %d messages from group %d", len(messages), groupID)

		// Сериализуем результат в JSON
		resultObj := map[string]interface{}{
			"messages": messages,
			"count":    len(messages),
			"group_id": groupID,
		}

		resultText, err := json.Marshal(resultObj)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to serialize result", err), nil
		}

		// Возвращаем результат
		return mcp.NewToolResultText(string(resultText)), nil
	}

	// Если мы здесь, значит все попытки не удались
	return mcp.NewToolResultErrorFromErr(
		fmt.Sprintf("Failed to get messages from group after %d attempts", maxRetries),
		lastErr), nil
}

// getInputPeerForGroup создает InputPeer для доступа к группе по ID
func (s *Server) getInputPeerForGroup(ctx context.Context, groupID int64) (tg.InputPeerClass, error) {
	log.Printf("Looking for group with ID: %d", groupID)

	// Check if we need to convert positive ID to negative for channels/supergroups
	// In Telegram API, channels and supergroups have negative IDs
	negativeID := groupID
	if groupID > 0 {
		negativeID = -groupID
		log.Printf("Converting positive ID %d to negative ID %d for channel/supergroup lookup", groupID, negativeID)
	}

	// Get available dialogs (chats, groups, channels)
	api := s.Client.API()
	dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dialogs: %w", err)
	}

	// We need to search through the returned chats for the group ID
	var foundChat tg.ChatClass
	var foundChannel tg.ChatClass

	// Extract chats and channels from the response
	var chats []tg.ChatClass
	var chatNames = make(map[string]tg.InputPeerClass) // Map title to input peer for search by name

	// Process based on the type of the response
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	case *tg.MessagesDialogsNotModified:
		return nil, fmt.Errorf("received MessagesDialogsNotModified, cannot extract chats")
	}

	// Extract channels and regular chats
	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Channel:
			// Try to match with both the original ID and the negative ID
			if c.ID == groupID || c.ID == negativeID {
				foundChannel = c
				log.Printf("Found channel match for ID %d (or %d): %s", groupID, negativeID, c.Title)
			}
			// Store for name search
			chatNames[c.Title] = &tg.InputPeerChannel{
				ChannelID:  c.ID,
				AccessHash: c.AccessHash,
			}
		case *tg.Chat:
			if c.ID == groupID || c.ID == negativeID {
				foundChat = c
				log.Printf("Found chat match for ID %d (or %d): %s", groupID, negativeID, c.Title)
			}
			// Store for name search
			chatNames[c.Title] = &tg.InputPeerChat{
				ChatID: c.ID,
			}
		case *tg.ChannelForbidden:
			if c.ID == groupID || c.ID == negativeID {
				// We know the ID but can't access it
				return nil, fmt.Errorf("found group %d but it's forbidden", groupID)
			}
		case *tg.ChatForbidden:
			if c.ID == groupID || c.ID == negativeID {
				return nil, fmt.Errorf("found chat %d but it's forbidden", groupID)
			}
		}
	}

	// If we found a channel or chat by ID, return the appropriate InputPeer
	if foundChannel != nil {
		channel, ok := foundChannel.(*tg.Channel)
		if ok {
			log.Printf("Found channel with ID: %d, Title: %s", channel.ID, channel.Title)
			return &tg.InputPeerChannel{
				ChannelID:  channel.ID,
				AccessHash: channel.AccessHash,
			}, nil
		}
	}

	if foundChat != nil {
		chat, ok := foundChat.(*tg.Chat)
		if ok {
			log.Printf("Found chat with ID: %d, Title: %s", chat.ID, chat.Title)
			return &tg.InputPeerChat{
				ChatID: chat.ID,
			}, nil
		}
	}

	// For channels and supergroups, Telegram API internally uses negative IDs
	// but the API might return positive IDs in group listings
	if groupID > 0 {
		// Try direct access with the ID as a channel ID but negative
		log.Printf("Group not found in dialogs, trying to create InputPeerChannel with ID: %d", negativeID)

		// For channels, we need both ID and access hash
		// Since we don't have access hash for this channel, this may or may not work
		// depending on whether the channel is public or private
		return &tg.InputPeerChannel{
			ChannelID:  negativeID,
			AccessHash: 0, // We don't have the access hash
		}, nil
	}

	// Not found by ID or name
	return nil, fmt.Errorf("invalid group ID: %d, group not found in available dialogs", groupID)
}
