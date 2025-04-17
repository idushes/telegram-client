package mcp

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
)

// registerTools регистрирует инструменты для взаимодействия с MCP сервером
func (s *Server) registerTools() {
	// Создаем определения инструментов только для PIN-кода
	sendCodeTool := mcp.NewTool("telegram_send_code",
		mcp.WithDescription("Send authentication code for Telegram"),
		mcp.WithString("code", mcp.Required()),
	)

	// Регистрируем инструмент с обработчиком
	s.MCPServer.AddTool(sendCodeTool, s.handleSendCode)
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
