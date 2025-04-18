package main

import (
	"flag"
	"fmt"
	"os"
)

// ParseConfig парсит параметры командной строки и переменные окружения
// и возвращает конфигурацию для авторизации в Telegram
func ParseConfig() (AuthConfig, error) {
	// Переопределяем функцию вывода справки
	flag.Usage = PrintUsage

	// Parse command line flags
	appID := flag.Int("app-id", 0, "Telegram app ID")
	appHash := flag.String("app-hash", "", "Telegram app hash")
	phone := flag.String("phone", "", "Phone number in international format")
	sessionFile := flag.String("session-file", "tg-session.json", "Path to session file")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	// Отображаем справку если запрошена
	if *help {
		PrintUsage()
		os.Exit(0)
	}

	// Check if values are provided via flags or environment variables
	if *appID == 0 {
		if envID := os.Getenv("APP_ID"); envID != "" {
			fmt.Sscanf(envID, "%d", appID)
		}
	}

	if *appHash == "" {
		*appHash = os.Getenv("APP_HASH")
	}

	if *phone == "" {
		*phone = os.Getenv("PHONE")
	}

	// Validate required credentials
	if *appID == 0 || *appHash == "" || *phone == "" {
		return AuthConfig{}, fmt.Errorf("required parameters missing: provide app-id, app-hash, and phone via flags or environment variables")
	}

	return AuthConfig{
		AppID:       *appID,
		AppHash:     *appHash,
		Phone:       *phone,
		SessionFile: *sessionFile,
	}, nil
}

// PrintUsage выводит информацию о использовании приложения
func PrintUsage() {
	fmt.Println("Telegram Authentication Client")
	fmt.Println("------------------------------")
	fmt.Println("A simple application that authenticates with Telegram, saves a session file, and exits.")
	fmt.Println("\nUsage of telegram-auth:")
	flag.PrintDefaults()

	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  APP_ID   - Telegram app ID")
	fmt.Println("  APP_HASH - Telegram app hash")
	fmt.Println("  PHONE    - Phone number in international format")

	fmt.Println("\nExamples:")
	fmt.Println("  Using command-line flags:")
	fmt.Println("    ./telegram-auth --app-id=12345 --app-hash=abcdef1234567890abcdef --phone=+1234567890")
	fmt.Println("\n  Using environment variables:")
	fmt.Println("    export APP_ID=12345")
	fmt.Println("    export APP_HASH=abcdef1234567890abcdef")
	fmt.Println("    export PHONE=+1234567890")
	fmt.Println("    ./telegram-auth")
}
