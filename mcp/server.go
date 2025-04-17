package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/mark3labs/mcp-go/server"
)

// Server представляет нашу реализацию MCP сервера для Telegram клиента
type Server struct {
	MCPServer   *server.MCPServer
	SSEServer   *server.SSEServer
	Client      *telegram.Client
	Code        string
	AuthState   string // может быть "none", "awaiting_code"
	AuthMutex   sync.Mutex
	CodeReady   chan struct{}
	PhoneNumber string
	AppID       int
	AppHash     string
	RetryDelay  time.Duration
	Port        string
	SessionID   string
}

// NewServer создает новый экземпляр Server
func NewServer(ctx context.Context) (*Server, error) {
	// Получаем порт из окружения
	port := os.Getenv("MCP_SERVER_PORT")
	if port == "" {
		return nil, errors.New("MCP_SERVER_PORT environment variable is not set")
	}

	// Получаем учетные данные Telegram из окружения
	phoneNumber := os.Getenv("PHONE")
	if phoneNumber == "" {
		phoneNumber = os.Getenv("TELEGRAM_PHONE") // Пробуем альтернативное имя
		if phoneNumber == "" {
			return nil, errors.New("PHONE environment variable is not set")
		}
	}

	appID := 0
	appIDStr := os.Getenv("APP_ID")
	if appIDStr == "" {
		appIDStr = os.Getenv("TELEGRAM_APP_ID") // Пробуем альтернативное имя
	}
	_, err := fmt.Sscanf(appIDStr, "%d", &appID)
	if err != nil || appID == 0 {
		return nil, errors.New("APP_ID environment variable is not set or invalid")
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		appHash = os.Getenv("TELEGRAM_APP_HASH") // Пробуем альтернативное имя
		if appHash == "" {
			return nil, errors.New("APP_HASH environment variable is not set")
		}
	}

	// Создаем MCP сервер
	mcpServer := server.NewMCPServer("telegram-client", "1.0.0")

	// Создаем SSE сервер
	sseServer := server.NewSSEServer(mcpServer)

	s := &Server{
		MCPServer:   mcpServer,
		SSEServer:   sseServer,
		AuthState:   "none",
		PhoneNumber: phoneNumber,
		AppID:       appID,
		AppHash:     appHash,
		RetryDelay:  5 * time.Second,
		Port:        port,
		CodeReady:   make(chan struct{}),
	}

	// Регистрируем инструменты для аутентификации
	s.registerTools()

	return s, nil
}

// Start запускает MCP сервер и Telegram клиент
func (s *Server) Start(ctx context.Context) error {
	// Настраиваем Telegram клиент
	if err := s.setupTelegramClient(ctx); err != nil {
		return err
	}

	// Запускаем MCP сервер с SSE
	log.Printf("Starting MCP server on port %s", s.Port)
	return s.SSEServer.Start("0.0.0.0:" + s.Port)
}

// SendNotification отправляет уведомление клиентам MCP
func (s *Server) SendNotification(method string, params map[string]interface{}) {
	// Просто логируем уведомление
	log.Printf("Notification: %s, Params: %v", method, params)
}
