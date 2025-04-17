package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// registerTools регистрирует инструменты для взаимодействия с MCP сервером
func (s *Server) registerTools() {
	// Регистрируем инструменты аутентификации
	s.registerAuthTools()

	// Регистрируем инструменты для групп
	s.registerGroupTools()
}

// registerAuthTools регистрирует инструменты для аутентификации
func (s *Server) registerAuthTools() {
	// Создаем определения инструментов для отправки кода авторизации
	sendCodeTool := mcp.NewTool("send_code",
		mcp.WithDescription("Send authentication code for Telegram"),
		mcp.WithString("code", mcp.Required()),
	)

	// Регистрируем инструмент с обработчиком
	s.MCPServer.AddTool(sendCodeTool, s.handleSendCode)
}

// registerGroupTools регистрирует инструменты для работы с группами
func (s *Server) registerGroupTools() {
	// Создаем инструмент для получения списка групп
	getGroupsTool := mcp.NewTool("get_groups",
		mcp.WithDescription("Get list of Telegram groups"),
		mcp.WithNumber("limit", mcp.DefaultNumber(50)),
	)

	// Регистрируем инструмент с обработчиком
	s.MCPServer.AddTool(getGroupsTool, s.handleGetGroups)
}
