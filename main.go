package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// codeAuthenticator implements auth.CodeAuthenticator
type codeAuthenticator struct{}

func (ca *codeAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter the code you received: ")
	var code string
	_, err := fmt.Scan(&code)
	return code, err
}

func main() {
	// Parse command line flags
	appID := flag.Int("app-id", 0, "Telegram app ID")
	appHash := flag.String("app-hash", "", "Telegram app hash")
	phone := flag.String("phone", "", "Phone number in international format")
	sessionFile := flag.String("session-file", "tg-session.json", "Path to session file")
	flag.Parse()

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
		fmt.Println("Error: Required parameters missing")
		fmt.Println("Please provide app-id, app-hash, and phone via flags or environment variables")
		flag.Usage()
		os.Exit(1)
	}

	// Convert to absolute path if necessary
	if !filepath.IsAbs(*sessionFile) {
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Printf("Failed to get current directory: %v\n", err)
			os.Exit(1)
		}
		*sessionFile = filepath.Join(currentDir, *sessionFile)
	}

	fmt.Printf("Using session file: %s\n", *sessionFile)

	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Create client
	client := telegram.NewClient(*appID, *appHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: *sessionFile,
		},
	})

	// Run client in a separate goroutine
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Setup auth flow with CodeOnly helper
			flow := auth.NewFlow(
				auth.CodeOnly(*phone, &codeAuthenticator{}),
				auth.SendCodeOptions{},
			)

			// Try to authorize
			fmt.Println("Authorizing...")
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				fmt.Printf("Authentication error: %v\n", err)
				os.Exit(1)
			}

			// Check successful authorization
			status, err := client.Auth().Status(ctx)
			if err != nil {
				fmt.Printf("Failed to get auth status: %v\n", err)
				os.Exit(1)
			}

			if !status.Authorized {
				fmt.Println("Failed to authorize. Check credentials and try again.")
				os.Exit(1)
			}

			fmt.Println("Successfully authenticated!")
			fmt.Printf("Session saved to: %s\n", *sessionFile)
			fmt.Println("Session file saved. Done!")

			// Signal we're done
			cancel()
			return nil
		})

		if err != nil {
			fmt.Printf("Error running client: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for the client to finish or for an interrupt
	<-ctx.Done()
}
