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
	MCPServer    *server.MCPServer
	SSEServer    *server.SSEServer
	Client       *telegram.Client
	Code         string
	AuthState    string // может быть "none", "awaiting_code"
	AuthMutex    sync.Mutex
	CodeReady    chan struct{}
	PhoneNumber  string
	AppID        int
	AppHash      string
	RetryDelay   time.Duration
	Port         string
	SessionID    string
	ETCDEndpoint string       // New field for ETCD endpoint
	ClientReady  bool         // Флаг готовности клиента
	ClientMutex  sync.RWMutex // Мьютекс для безопасного доступа к клиенту
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

	// Проверяем наличие ETCD endpoint
	etcdEndpoint := os.Getenv("ETCD_ENDPOINT")

	// Создаем MCP сервер
	mcpServer := server.NewMCPServer("telegram-client", "1.0.0")

	// Создаем SSE сервер
	sseServer := server.NewSSEServer(mcpServer)

	s := &Server{
		MCPServer:    mcpServer,
		SSEServer:    sseServer,
		AuthState:    "none",
		PhoneNumber:  phoneNumber,
		AppID:        appID,
		AppHash:      appHash,
		RetryDelay:   5 * time.Second,
		Port:         port,
		CodeReady:    make(chan struct{}),
		ETCDEndpoint: etcdEndpoint, // Сохраняем ETCD endpoint
	}

	// Регистрируем инструменты для аутентификации
	s.registerTools()

	return s, nil
}

// Start запускает MCP сервер и Telegram клиент
func (s *Server) Start(ctx context.Context) error {
	// Контекст с отменой для клиента
	clientCtx, clientCancel := context.WithCancel(ctx)

	// Настраиваем Telegram клиент
	if err := s.setupTelegramClient(clientCtx); err != nil {
		clientCancel()
		return err
	}

	// Запускаем мониторинг состояния клиента
	go s.monitorClientStatus(clientCtx)

	// Запускаем MCP сервер с SSE
	log.Printf("Starting MCP server on port %s", s.Port)
	return s.SSEServer.Start("0.0.0.0:" + s.Port)
}

// monitorClientStatus следит за состоянием клиента и пытается переподключиться при необходимости
func (s *Server) monitorClientStatus(ctx context.Context) {
	// Более частые проверки для быстрого реагирования на проблемы
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Счетчик последовательных ошибок для агрессивного переподключения
	consecErrorCount := 0
	const maxConsecErrors = 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Проверяем состояние клиента
			if s.Client == nil {
				log.Println("Client is nil, attempting to recreate")
				if err := s.setupTelegramClient(ctx); err != nil {
					log.Printf("Failed to recreate client: %v", err)
					consecErrorCount++
				}
				continue
			}

			// Создаем контекст с таймаутом для проверки
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

			// Проверяем состояние авторизации
			status, err := s.Client.Auth().Status(checkCtx)
			cancel() // Освобождаем ресурсы контекста сразу после использования

			if err != nil {
				log.Printf("Client status check failed: %v", err)
				consecErrorCount++

				// Если ошибка критическая - пересоздаем клиент сразу
				if consecErrorCount >= maxConsecErrors || isFatalClientError(err) {
					log.Printf("Critical condition detected (errors: %d/%d), recreating client immediately",
						consecErrorCount, maxConsecErrors)
					s.ClientMutex.Lock()
					s.ClientReady = false
					s.ClientMutex.Unlock()

					// Пытаемся пересоздать клиент
					if err := s.setupTelegramClient(ctx); err != nil {
						log.Printf("Failed to recreate client: %v", err)
					} else {
						consecErrorCount = 0
					}
				} else {
					// Для некритических ошибок просто помечаем клиент как неготовый
					s.ClientMutex.Lock()
					s.ClientReady = false
					s.ClientMutex.Unlock()
				}
			} else if status.Authorized {
				// Клиент в порядке, сбрасываем счетчик ошибок
				if consecErrorCount > 0 {
					log.Printf("Client status check successful after %d failures", consecErrorCount)
				}
				consecErrorCount = 0
				s.ClientMutex.Lock()
				s.ClientReady = true
				s.ClientMutex.Unlock()
			} else {
				// Клиент не авторизован
				log.Println("Client is not authorized")
				s.ClientMutex.Lock()
				s.ClientReady = false
				s.ClientMutex.Unlock()
				consecErrorCount++

				// Если долго не авторизован, пересоздаем
				if consecErrorCount >= maxConsecErrors {
					log.Printf("Client not authorized for %d consecutive checks, recreating", consecErrorCount)
					if err := s.setupTelegramClient(ctx); err != nil {
						log.Printf("Failed to recreate unauthorized client: %v", err)
					}
				}
			}
		}
	}
}

// checkClientStatus проверяет состояние клиента и пытается восстановить его при необходимости
// Эта функция вызывается по запросу из обработчиков
func (s *Server) checkClientStatus(ctx context.Context) {
	log.Println("Manual client status check initiated...")

	if s.Client == nil {
		log.Println("Client is nil, attempting to recreate")
		if err := s.setupTelegramClient(ctx); err != nil {
			log.Printf("Failed to recreate client: %v", err)
		}
		return
	}

	// Создаем контекст с таймаутом для проверки
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Проверяем состояние авторизации
	status, err := s.Client.Auth().Status(checkCtx)
	if err != nil {
		log.Printf("Manual client status check failed: %v", err)

		// Если ошибка критическая - пересоздаем клиент сразу
		if isFatalClientError(err) {
			log.Println("Fatal client error detected, recreating client immediately")
			s.ClientMutex.Lock()
			s.ClientReady = false
			s.ClientMutex.Unlock()

			// Пытаемся пересоздать клиент
			if err := s.setupTelegramClient(ctx); err != nil {
				log.Printf("Failed to recreate client: %v", err)
			}
		} else {
			// Для других ошибок, устанавливаем флаг неготовности
			log.Println("Non-fatal client error, marking client as not ready")
			s.ClientMutex.Lock()
			s.ClientReady = false
			s.ClientMutex.Unlock()
		}
	} else {
		// Проверяем статус авторизации
		if status.Authorized {
			log.Println("Manual client status check passed, client is authorized and ready")
			s.ClientMutex.Lock()
			s.ClientReady = true
			s.ClientMutex.Unlock()
		} else {
			log.Println("Client is not authorized")
			s.ClientMutex.Lock()
			s.ClientReady = false
			s.ClientMutex.Unlock()
		}
	}
}

// IsClientReady проверяет, готов ли клиент к использованию
func (s *Server) IsClientReady() bool {
	s.ClientMutex.RLock()
	defer s.ClientMutex.RUnlock()
	return s.ClientReady
}

// SendNotification отправляет уведомление клиентам MCP
func (s *Server) SendNotification(method string, params map[string]interface{}) {
	// Просто логируем уведомление
	log.Printf("Notification: %s, Params: %v", method, params)
}
