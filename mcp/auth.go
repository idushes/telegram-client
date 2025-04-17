package mcp

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// setupTelegramClient настраивает Telegram клиент и поток авторизации
func (s *Server) setupTelegramClient(ctx context.Context) error {
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

	// Создаем Telegram клиент с хранилищем сессии
	client := telegram.NewClient(s.AppID, s.AppHash, telegram.Options{
		SessionStorage: sessionStorage,
	})
	s.Client = client

	// Запускаем клиент в фоновом режиме
	go func() {
		// Запускаем клиент до завершения или ошибки
		if err := client.Run(ctx, func(ctx context.Context) error {
			// Инициализируем попытку авторизации
			return s.attemptAuth(ctx)
		}); err != nil {
			// Проверяем тип ошибки
			if s.ETCDEndpoint != "" && isETCDConnectionError(err) {
				// Завершаем приложение при ошибке соединения с ETCD
				log.Fatalf("Fatal ETCD connection error: %v", err)
			} else {
				// Для других ошибок (в том числе отсутствие ключа сессии) просто логируем и продолжаем
				log.Printf("Telegram client error: %v", err)
			}
		}
	}()

	return nil
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

// waitForCode ожидает код авторизации
func (s *Server) waitForCode(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	// Блокируем для установки состояния авторизации
	s.AuthMutex.Lock()
	s.AuthState = "awaiting_code"
	s.AuthMutex.Unlock()

	// Отправляем уведомление
	s.SendNotification("telegram_auth_code_needed", map[string]interface{}{
		"phone": s.PhoneNumber,
		"type":  sentCode.Type.TypeName(),
	})

	// Выводим информацию
	log.Printf("Telegram authentication code needed for phone: %s", s.PhoneNumber)
	log.Printf("Authentication code type: %s", sentCode.Type.TypeName())
	log.Printf("To provide the code, use the telegram_send_code tool with {\"code\": \"YOUR_CODE\"}")

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
				s.SendNotification("telegram_auth_error", map[string]interface{}{
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
		s.SendNotification("telegram_auth_success", map[string]interface{}{
			"success": true,
		})

		return nil
	}
}
