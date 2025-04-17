package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/mark3labs/mcp-go/mcp"
)

// registerTools регистрирует инструменты для взаимодействия с MCP сервером
func (s *Server) registerTools() {
	// Создаем определения инструментов только для PIN-кода
	sendCodeTool := mcp.NewTool("telegram_send_code",
		mcp.WithDescription("Send authentication code for Telegram"),
		mcp.WithString("code", mcp.Required()),
	)

	// Создаем инструмент для получения списка групп
	getGroupsTool := mcp.NewTool("telegram_get_groups",
		mcp.WithDescription("Get list of Telegram groups"),
		mcp.WithNumber("limit", mcp.DefaultNumber(50)),
	)

	// Регистрируем инструменты с обработчиками
	s.MCPServer.AddTool(sendCodeTool, s.handleSendCode)
	s.MCPServer.AddTool(getGroupsTool, s.handleGetGroups)
}

// handleSendCode обрабатывает запрос на отправку кода авторизации
func (s *Server) handleSendCode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.AuthMutex.Lock()
	defer s.AuthMutex.Unlock()

	// Разрешаем отправку кода только если мы ждем код
	if s.AuthState != "awaiting_code" {
		return mcp.NewToolResultErrorFromErr("Authentication code not requested or already provided", errors.New("invalid state")), nil
	}

	// Извлекаем код из параметров
	codeParam, ok := req.Params.Arguments["code"]
	if !ok {
		return mcp.NewToolResultErrorFromErr("Missing code parameter", errors.New("code parameter required")), nil
	}

	codeStr, ok := codeParam.(string)
	if !ok {
		return mcp.NewToolResultErrorFromErr("Invalid code format", errors.New("code must be a string")), nil
	}

	// Сохраняем код и сигнализируем о его готовности
	s.Code = codeStr
	s.AuthState = "none"
	close(s.CodeReady)
	s.CodeReady = make(chan struct{})

	return mcp.NewToolResultText("Code accepted"), nil
}

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
	limit := 50
	if limitParam, ok := req.Params.Arguments["limit"]; ok {
		if limitValue, ok := limitParam.(float64); ok {
			limit = int(limitValue)
		}
	}

	log.Printf("Getting list of Telegram groups with limit %d", limit)

	// Создаем контекст с таймаутом для запроса
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Создаем API клиент
	api := s.Client.API()

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
			return mcp.NewToolResultErrorFromErr("Telegram connection error detected. System is reconnecting, please try again in a few seconds.", err), nil
		}

		// Запускаем проверку состояния клиента
		go s.checkClientStatus(context.Background())

		return mcp.NewToolResultErrorFromErr("Failed to check auth status, please try again later", err), nil
	case authorized := <-authStatus:
		if !authorized {
			log.Printf("User not authorized")
			return mcp.NewToolResultErrorFromErr("User not authorized", errors.New("authentication required")), nil
		}
	case <-reqCtx.Done():
		log.Printf("Auth status check timed out")

		// Устанавливаем флаг неготовности клиента
		s.ClientMutex.Lock()
		s.ClientReady = false
		s.ClientMutex.Unlock()

		// Запускаем проверку состояния клиента
		go s.checkClientStatus(context.Background())

		return mcp.NewToolResultErrorFromErr("Auth status check timed out, please try again later", reqCtx.Err()), nil
	}

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
			Limit:      limit,
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
				}
			}
		case *tg.MessagesDialogsSlice:
			// Фильтруем только группы из списка диалогов
			for _, chat := range d.Chats {
				group := extractGroupInfo(chat)
				if group != nil {
					groups = append(groups, group)
				}
			}
		default:
			log.Printf("Unknown dialogs type: %T", dialogs)
			return mcp.NewToolResultErrorFromErr("Unknown dialogs response type", errors.New("unexpected response type")), nil
		}

		log.Printf("Found %d groups", len(groups))

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

// isFatalClientError теперь определена в файле auth.go
