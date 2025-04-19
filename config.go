package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
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
	// CommandChats команда получения списка чатов
	CommandChats CommandType = "chats"
	// CommandMessages команда получения сообщений из чата
	CommandMessages CommandType = "messages"
	// CommandUnknown неизвестная команда
	CommandUnknown CommandType = "unknown"
)

// Config содержит команду и параметры приложения
type Config struct {
	Command    CommandType
	AuthConfig AuthConfig
	ChatID     int64 // ID чата для команды messages
	Limit      int   // Ограничение на количество сообщений
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

	// Если это команда login или chats, нужно парсить аргументы
	if command == CommandSignIn || command == CommandChats {
		// Создаем новый набор флагов для аргументов
		authFlags := flag.NewFlagSet(string(command), flag.ExitOnError)
		appID := authFlags.Int("app-id", 0, "Telegram app ID")
		appHash := authFlags.String("app-hash", "", "Telegram app hash")
		phone := authFlags.String("phone", "", "Phone number in international format")
		sessionFile := authFlags.String("session-file", "tg-session.json", "Path to session file")
		help := authFlags.Bool("help", false, "Show help for command")

		// Парсим аргументы после команды
		err := authFlags.Parse(os.Args[2:])
		if err != nil {
			return Config{Command: command}, err
		}

		// Если запрошена справка
		if *help {
			if command == CommandSignIn {
				printSignInHelp(authFlags)
			} else if command == CommandChats {
				printChatsHelp(authFlags)
			}
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
			if command == CommandSignIn {
				printSignInHelp(authFlags)
			} else if command == CommandChats {
				printChatsHelp(authFlags)
			}
			return Config{Command: command}, fmt.Errorf("required parameters missing: provide app-id, app-hash, and phone via flags or environment variables")
		}

		// Создаем и возвращаем конфигурацию
		return Config{
			Command: command,
			AuthConfig: AuthConfig{
				AppID:       *appID,
				AppHash:     *appHash,
				Phone:       *phone,
				SessionFile: *sessionFile,
			},
		}, nil
	}

	// Если это команда messages
	if command == CommandMessages {
		// Создаем новый набор флагов для аргументов
		messagesFlags := flag.NewFlagSet(string(command), flag.ExitOnError)
		appID := messagesFlags.Int("app-id", 0, "Telegram app ID")
		appHash := messagesFlags.String("app-hash", "", "Telegram app hash")
		phone := messagesFlags.String("phone", "", "Phone number in international format")
		sessionFile := messagesFlags.String("session-file", "tg-session.json", "Path to session file")
		chatID := messagesFlags.Int64("chat-id", 0, "Chat ID to get messages from")
		limit := messagesFlags.Int("limit", 20, "Maximum number of messages to retrieve")
		help := messagesFlags.Bool("help", false, "Show help for command")

		// Парсим аргументы после команды
		err := messagesFlags.Parse(os.Args[2:])
		if err != nil {
			return Config{Command: command}, err
		}

		// Если запрошена справка
		if *help {
			printMessagesHelp(messagesFlags)
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

		// Проверяем chat-id из переменной окружения
		if *chatID == 0 {
			if envChatID := os.Getenv("CHAT_ID"); envChatID != "" {
				chatIDVal, err := strconv.ParseInt(envChatID, 10, 64)
				if err == nil {
					*chatID = chatIDVal
				}
			}
		}

		// Проверяем, что все необходимые параметры заданы
		if *appID == 0 || *appHash == "" || *phone == "" {
			printMessagesHelp(messagesFlags)
			return Config{Command: command}, fmt.Errorf("required parameters missing: provide app-id, app-hash, and phone via flags or environment variables")
		}

		// Проверяем, что указан chat-id
		if *chatID == 0 {
			printMessagesHelp(messagesFlags)
			return Config{Command: command}, fmt.Errorf("chat-id is required")
		}

		// Создаем и возвращаем конфигурацию
		return Config{
			Command: command,
			AuthConfig: AuthConfig{
				AppID:       *appID,
				AppHash:     *appHash,
				Phone:       *phone,
				SessionFile: *sessionFile,
			},
			ChatID: *chatID,
			Limit:  *limit,
		}, nil
	}

	// Неизвестная команда
	return Config{Command: CommandUnknown}, fmt.Errorf("unknown command: %s", command)
}

// PrintHelp выводит общую справку по приложению
func PrintHelp() {
	fmt.Println("Telegram Authentication Client")
	fmt.Println("------------------------------")
	fmt.Println("A simple application that authenticates with Telegram, saves a session file, and exits.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  login      Authenticate with Telegram and save session file")
	fmt.Println("  chats      Get list of all chats in JSON format")
	fmt.Println("  messages   Get messages from a specific chat in JSON format")
	fmt.Println("  help       Display this help message")
	fmt.Println("  test       Run a test to check if application works properly")
	fmt.Println("\nExamples:")
	fmt.Println("  Sign in with command-line flags:")
	fmt.Println("    ./telegram-auth login --app-id=12345 --app-hash=abcdef1234567890abcdef --phone=+1234567890")
	fmt.Println("\n  Get chats with environment variables:")
	fmt.Println("    export APP_ID=12345")
	fmt.Println("    export APP_HASH=abcdef1234567890abcdef")
	fmt.Println("    export PHONE=+1234567890")
	fmt.Println("    ./telegram-auth chats")
	fmt.Println("\n  Get messages from a chat:")
	fmt.Println("    ./telegram-auth messages --chat-id=-1001234567890 --limit=50")
	fmt.Println("\n  Show help for login command:")
	fmt.Println("    ./telegram-auth login --help")
}

// printSignInHelp выводит справку по команде login
func printSignInHelp(fs *flag.FlagSet) {
	fmt.Println("Telegram Authentication Client - Login")
	fmt.Println("-------------------------------------")
	fmt.Println("Authenticate with Telegram and save session file.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth login [options]")
	fmt.Println("\nOptions:")
	fs.PrintDefaults()
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  APP_ID   - Telegram app ID")
	fmt.Println("  APP_HASH - Telegram app hash")
	fmt.Println("  PHONE    - Phone number in international format")
}

// printChatsHelp выводит справку по команде chats
func printChatsHelp(fs *flag.FlagSet) {
	fmt.Println("Telegram Authentication Client - Chats")
	fmt.Println("------------------------------------")
	fmt.Println("Get list of all chats in JSON format.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth chats [options]")
	fmt.Println("\nOptions:")
	fs.PrintDefaults()
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  APP_ID   - Telegram app ID")
	fmt.Println("  APP_HASH - Telegram app hash")
	fmt.Println("  PHONE    - Phone number in international format")
}

// printMessagesHelp выводит справку по команде messages
func printMessagesHelp(fs *flag.FlagSet) {
	fmt.Println("Telegram Authentication Client - Messages")
	fmt.Println("---------------------------------------")
	fmt.Println("Get messages from a specific chat in JSON format.")
	fmt.Println("\nUsage:")
	fmt.Println("  telegram-auth messages [options]")
	fmt.Println("\nOptions:")
	fs.PrintDefaults()
	fmt.Println("\nEnvironment Variables:")
	fmt.Println("  APP_ID   - Telegram app ID")
	fmt.Println("  APP_HASH - Telegram app hash")
	fmt.Println("  PHONE    - Phone number in international format")
	fmt.Println("  CHAT_ID  - Chat ID to get messages from")
	fmt.Println("\nNotes:")
	fmt.Println("  - Chat ID is required and must be specified via --chat-id flag or CHAT_ID environment variable")
	fmt.Println("  - Use the 'chats' command to get the list of available chats and their IDs")
	fmt.Println("  - Chat IDs for groups and channels are usually negative numbers")
}
