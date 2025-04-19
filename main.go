package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func main() {
	// Парсим конфигурацию с командами и параметрами
	config, err := ParseConfig()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Выполняем действие в зависимости от команды
	switch config.Command {
	case CommandSignIn:
		// Авторизация в Telegram
		if err := runSignIn(config.AuthConfig); err != nil {
			fmt.Printf("Authentication failed: %v\n", err)
			os.Exit(1)
		}
	case CommandChats:
		// Получение списка чатов
		if err := runChats(config.AuthConfig); err != nil {
			fmt.Printf("Failed to get chats: %v\n", err)
			os.Exit(1)
		}
	case CommandMessages:
		// Получение сообщений из чата
		if err := runMessages(config.AuthConfig, config.ChatID, config.Limit); err != nil {
			fmt.Printf("Failed to get messages: %v\n", err)
			os.Exit(1)
		}
	case CommandEvents:
		// Отслеживание событий Telegram
		if err := runEvents(config.AuthConfig, config.Timeout); err != nil {
			fmt.Printf("Failed to track events: %v\n", err)
			os.Exit(1)
		}
	case CommandHelp:
		// Показать справку
		PrintHelp()
	case CommandTest:
		// Выполнить тестовую команду
		fmt.Println("Test passed")
	default:
		// Неизвестная команда
		fmt.Printf("Unknown command: %s\n", config.Command)
		PrintHelp()
		os.Exit(1)
	}
}

// runSignIn выполняет авторизацию в Telegram
func runSignIn(authConfig AuthConfig) error {
	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Run authentication
	return Authenticate(ctx, authConfig)
}

// runChats выполняет получение списка чатов
func runChats(authConfig AuthConfig) error {
	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Run chats retrieval
	return GetChats(ctx, authConfig)
}

// runMessages выполняет получение сообщений из чата
func runMessages(authConfig AuthConfig, chatID int64, limit int) error {
	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Run messages retrieval
	return GetMessages(ctx, authConfig, chatID, limit)
}

// runEvents выполняет отслеживание событий Telegram
func runEvents(authConfig AuthConfig, timeout int) error {
	// Создаем контекст с обработкой сигналов
	baseCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Если указан таймаут, создаем контекст с ограничением по времени
	var ctx context.Context
	if timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(baseCtx, time.Duration(timeout)*time.Second)
		defer timeoutCancel()
	} else {
		ctx = baseCtx
	}

	// Запускаем отслеживание событий
	return GetEvents(ctx, authConfig, timeout)
}
