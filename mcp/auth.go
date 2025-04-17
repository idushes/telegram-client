package mcp

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// setupTelegramClient настраивает Telegram клиент и поток авторизации
func (s *Server) setupTelegramClient(ctx context.Context) error {
	// Если клиент уже существует, дадим время для завершения операций
	if s.Client != nil {
		log.Println("Cleaning up existing client before creating a new one")

		// Небольшая пауза для завершения всех операций
		time.Sleep(2 * time.Second)

		// Убираем старый клиент
		s.Client = nil

		// Принудительный сбор мусора для освобождения ресурсов
		runtime.GC()
		time.Sleep(1 * time.Second)
	}

	// Сбрасываем флаг готовности клиента
	s.ClientMutex.Lock()
	s.ClientReady = false
	s.ClientMutex.Unlock()

	// Выводим информацию о конфигурации
	log.Printf("Setting up Telegram client with:")
	log.Printf("  Phone number: %s", s.PhoneNumber)
	log.Printf("  App ID: %d", s.AppID)
	log.Printf("  App hash: %s", s.AppHash)

	// Создаем хеш телефона для имени файла сессии
	phoneHash := fmt.Sprintf("%x", md5.Sum([]byte(s.PhoneNumber)))

	// Создаем и настраиваем хранилище сессии
	var sessionStorage telegram.SessionStorage
	var err error

	if s.ETCDEndpoint != "" {
		// Используем ETCD хранилище
		log.Printf("Using ETCD session storage at %s", s.ETCDEndpoint)
		sessionStorage, err = NewETCDSessionStorage(s.ETCDEndpoint, phoneHash)
		if err != nil {
			// Завершаем приложение при ошибке соединения с ETCD
			log.Fatalf("Failed to create ETCD session storage: %v", err)
			return fmt.Errorf("fatal: failed to create ETCD session storage: %w", err)
		}
	} else {
		// Используем файловое хранилище
		log.Printf("Using file-based session storage")
		// Создаем директорию сессии
		sessionDir := filepath.Join(".", "session")
		if err := os.MkdirAll(sessionDir, 0700); err != nil {
			return fmt.Errorf("failed to create session directory: %w", err)
		}

		// Создаем путь к файлу сессии
		sessionFile := filepath.Join(sessionDir, fmt.Sprintf("%s.session", phoneHash))
		log.Printf("Storing session in file: %s", sessionFile)

		// Создаем хранилище сессии
		sessionStorage = &telegram.FileSessionStorage{
			Path: sessionFile,
		}
	}

	// Создаем улучшенные опции подключения
	opts := telegram.Options{
		SessionStorage: sessionStorage,
		RetryInterval:  2 * time.Second,
		MaxRetries:     5,
		Middlewares:    []telegram.Middleware{},
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, updates tg.UpdatesClass) error {
			go s.handleTelegramUpdates(context.Background(), updates)
			return nil
		}),
	}

	// Создаем Telegram клиент с хранилищем сессии и улучшенными опциями
	client := telegram.NewClient(s.AppID, s.AppHash, opts)
	s.Client = client

	// Запускаем клиент в фоновом режиме
	go func() {
		// Создаем собственный контекст для клиента с возможностью отмены
		clientCtx, clientCancel := context.WithCancel(ctx)
		defer clientCancel()

		// Устанавливаем таймаут для инициализации
		initCtx, initCancel := context.WithTimeout(clientCtx, 30*time.Second)
		defer initCancel()

		// Запускаем клиент до завершения или ошибки
		if err := client.Run(initCtx, func(ctx context.Context) error {
			// Инициализируем попытку авторизации
			err := s.attemptAuth(ctx)
			if err == nil {
				// Если авторизация успешна, устанавливаем флаг готовности
				s.ClientMutex.Lock()
				s.ClientReady = true
				s.ClientMutex.Unlock()
				log.Println("Client is now ready")

				// После успешной авторизации, переключаемся на длительный контекст
				<-clientCtx.Done() // Ожидаем завершения основного контекста
				return nil
			}
			return err
		}); err != nil {
			// Сбрасываем флаг готовности при ошибке
			s.ClientMutex.Lock()
			s.ClientReady = false
			s.ClientMutex.Unlock()

			// Проверяем тип ошибки
			if s.ETCDEndpoint != "" && isETCDConnectionError(err) {
				// Завершаем приложение при ошибке соединения с ETCD
				log.Fatalf("Fatal ETCD connection error: %v", err)
			} else if isFatalClientError(err) {
				// Для критических ошибок пересоздаем клиент
				log.Printf("Fatal client error detected: %v, recreating client...", err)
				// Создаем новый контекст для пересоздания клиента
				newCtx := context.Background()
				go func() {
					// Добавляем задержку перед пересозданием
					time.Sleep(5 * time.Second)
					if err := s.setupTelegramClient(newCtx); err != nil {
						log.Printf("Failed to recreate client: %v", err)
					}
				}()
			} else {
				// Для других ошибок просто логируем и продолжаем
				log.Printf("Telegram client error: %v", err)
			}
		}
	}()

	return nil
}

// customReconnectionPolicy реализует политику переподключения с экспоненциальной задержкой
type customReconnectionPolicy struct {
	attempt int
	maxWait time.Duration
	base    time.Duration
}

// NextRetry реализует логику задержки между попытками переподключения
func (p *customReconnectionPolicy) NextRetry() time.Duration {
	p.attempt++

	// Экспоненциальное увеличение задержки с учетом случайности
	jitter := time.Duration(rand.Int63n(int64(p.base)))
	delay := (p.base * time.Duration(1<<uint(p.attempt-1))) + jitter

	// Ограничиваем максимальное время ожидания
	if delay > p.maxWait {
		delay = p.maxWait
	}

	log.Printf("Reconnection attempt %d, waiting for %v", p.attempt, delay)
	return delay
}

// Reset сбрасывает счетчик попыток при успешном соединении
func (p *customReconnectionPolicy) Reset() {
	log.Println("Resetting reconnection policy")
	p.attempt = 0
}

// isETCDConnectionError определяет, является ли ошибка проблемой подключения к ETCD
func isETCDConnectionError(err error) bool {
	errStr := err.Error()

	// Проверяем строки ошибок, которые указывают на проблемы с соединением
	connectionErrors := []string{
		"connection refused",
		"dial tcp",
		"no route to host",
		"i/o timeout",
		"failed to send HTTP request to ETCD",
	}

	// Исключаем ошибки отсутствия ключа, которые являются нормальными
	if errors.Is(err, session.ErrNotFound) {
		return false
	}

	// Проверяем наличие маркеров ошибки соединения
	for _, connErr := range connectionErrors {
		if strings.Contains(errStr, connErr) {
			return true
		}
	}

	return false
}

// isFatalClientError определяет, является ли ошибка критической для клиента
// и требует пересоздания клиента
func isFatalClientError(err error) bool {
	errStr := err.Error()

	fatalErrors := []string{
		"engine was closed",
		"connection dead",
		"waitSession",
		"connection closed",
		"failed to connect",
		"i/o timeout",
		"broken pipe",
		"EOF",
		"context canceled",
		"no such host",
		"network is unreachable",
	}

	for _, e := range fatalErrors {
		if strings.Contains(errStr, e) {
			return true
		}
	}

	return false
}

// waitForCode ожидает код авторизации
func (s *Server) waitForCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	// Блокируем для установки состояния авторизации
	s.AuthMutex.Lock()
	s.AuthState = "awaiting_code"
	s.AuthMutex.Unlock()

	// Отправляем уведомление
	s.SendNotification("auth_code_needed", map[string]interface{}{
		"phone": s.PhoneNumber,
		"type":  sentCode.Type.TypeName(),
	})

	// Выводим информацию
	log.Printf("Telegram authentication code needed for phone: %s", s.PhoneNumber)
	log.Printf("Authentication code type: %s", sentCode.Type.TypeName())
	log.Printf("To provide the code, use the send_code tool with {\"code\": \"YOUR_CODE\"}")

	// Ждем пока код будет предоставлен через вызов инструмента
	select {
	case <-s.CodeReady:
		log.Printf("Authentication code received: %s", s.Code)
		return s.Code, nil
	case <-ctx.Done():
		log.Printf("Context cancelled while waiting for authentication code")
		return "", ctx.Err()
	}
}

// attemptAuth пытается выполнить авторизацию в Telegram
func (s *Server) attemptAuth(ctx context.Context) error {
	for {
		flow := auth.NewFlow(
			auth.CodeOnly(
				s.PhoneNumber,
				auth.CodeAuthenticatorFunc(s.waitForCode),
			),
			auth.SendCodeOptions{},
		)

		if err := s.Client.Auth().IfNecessary(ctx, flow); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("Authentication error: %v", err)

				// Отправляем уведомление об ошибке
				s.SendNotification("auth_error", map[string]interface{}{
					"error": err.Error(),
				})

				log.Printf("Retrying in %s...", s.RetryDelay)
				time.Sleep(s.RetryDelay)
				continue
			}
			return err
		}

		log.Println("Authentication successful!")

		// Отправляем уведомление об успешной авторизации
		s.SendNotification("auth_success", map[string]interface{}{
			"success": true,
		})

		return nil
	}
}
