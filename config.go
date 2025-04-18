package main

import (
	"flag"
	"fmt"
	"os"
)

// CommandType представляет тип команды
type CommandType string

const (
	// CommandSignIn команда авторизации
	CommandSignIn CommandType = "login"
	// CommandHelp команда вызова справки
	CommandHelp CommandType = "help"
	// CommandTest тестовая команда
	CommandTest CommandType = "test"
	// CommandUnknown неизвестная команда
	CommandUnknown CommandType = "unknown"
)

// Config содержит команду и параметры приложения
type Config struct {
	Command    CommandType
	AuthConfig AuthConfig
}

// ParseConfig парсит команды и параметры командной строки
func ParseConfig() (Config, error) {
	if len(os.Args) < 2 {
		return Config{Command: CommandUnknown}, fmt.Errorf("command required")
	}

	// Получаем команду из первого аргумента
	command := CommandType(os.Args[1])

	// Если это команда help, сразу возвращаем
	if command == CommandHelp {
		return Config{Command: CommandHelp}, nil
	}

	// Если это команда test, сразу возвращаем
	if command == CommandTest {
		return Config{Command: CommandTest}, nil
	}

	// Проверяем, что команда sign-in
	if command != CommandSignIn {
		return Config{Command: CommandUnknown}, fmt.Errorf("unknown command: %s", command)
	}

	// Для команды sign-in нужно парсить аргументы авторизации
	// Создаем новый набор флагов, чтобы парсить только аргументы после команды
	authFlags := flag.NewFlagSet("sign-in", flag.ExitOnError)
	appID := authFlags.Int("app-id", 0, "Telegram app ID")
	appHash := authFlags.String("app-hash", "", "Telegram app hash")
	phone := authFlags.String("phone", "", "Phone number in international format")
	sessionFile := authFlags.String("session-file", "tg-session.json", "Path to session file")
	help := authFlags.Bool("help", false, "Show help for sign-in command")

	// Парсим аргументы после команды
	err := authFlags.Parse(os.Args[2:])
	if err != nil {
		return Config{Command: CommandSignIn}, err
	}

	// Если запрошена справка sign-in
	if *help {
		printSignInHelp(authFlags)
		os.Exit(0)
	}

	// Проверяем переменные окружения
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

	// Проверяем, что все необходимые параметры заданы
	if *appID == 0 || *appHash == "" || *phone == "" {
		printSignInHelp(authFlags)
		return Config{Command: CommandSignIn}, fmt.Errorf("required parameters missing: provide app-id, app-hash, and phone via flags or environment variables")
	}

	// Создаем и возвращаем конфигурацию
	return Config{
		Command: CommandSignIn,
		AuthConfig: AuthConfig{
			AppID:       *appID,
			AppHash:     *appHash,
			Phone:       *phone,
			SessionFile: *sessionFile,
		},
	}, nil
}

// PrintHelp выводит общую справку по приложению
func PrintHelp() {
	fmt.Println("Telegram Authentication Client")
	fmt.Println("------------------------------")
	fmt.Println("A simple application that authenticates with Telegram, saves a session file, and exits.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  sign-in    Authenticate with Telegram and save session file")
	fmt.Println("  help       Display this help message")
	fmt.Println("  test       Run a test to check if application works properly")
	fmt.Println("\nExamples:")
	fmt.Println("  Sign in with command-line flags:")
	fmt.Println("    ./telegram-auth sign-in --app-id=12345 --app-hash=abcdef1234567890abcdef --phone=+1234567890")
	fmt.Println("\n  Sign in with environment variables:")
	fmt.Println("    export APP_ID=12345")
	fmt.Println("    export APP_HASH=abcdef1234567890abcdef")
	fmt.Println("    export PHONE=+1234567890")
	fmt.Println("    ./telegram-auth sign-in")
	fmt.Println("\n  Show help for sign-in command:")
	fmt.Println("    ./telegram-auth sign-in --help")
}

// printSignInHelp выводит справку по команде sign-in
func printSignInHelp(fs *flag.FlagSet) {
	fmt.Println("Telegram Authentication Client - Sign In")
	fmt.Println("---------------------------------------")
	fmt.Println("Authenticate with Telegram and save session file.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth sign-in [options]")
	fmt.Println("\nOptions:")
	fs.PrintDefaults()
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  APP_ID   - Telegram app ID")
	fmt.Println("  APP_HASH - Telegram app hash")
	fmt.Println("  PHONE    - Phone number in international format")
}
