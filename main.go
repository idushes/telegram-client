package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
