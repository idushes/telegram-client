package mcp

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

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

	// Создаем директорию сессии
	sessionDir := filepath.Join(".", "session")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Создаем путь к файлу сессии
	sessionFile := filepath.Join(sessionDir, fmt.Sprintf("%s.session", phoneHash))
	log.Printf("Storing session in file: %s", sessionFile)

	// Создаем хранилище сессии
	sessionStorage := &telegram.FileSessionStorage{
		Path: sessionFile,
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
			log.Printf("Telegram client error: %v", err)
		}
	}()

	return nil
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
