package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// telegramCodeAuth implements auth.CodeAuthenticator
type telegramCodeAuth struct{}

func (tca *telegramCodeAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter the code you received: ")
	var code string
	_, err := fmt.Scan(&code)
	return code, err
}

// AuthConfig содержит конфигурацию для авторизации
type AuthConfig struct {
	AppID       int
	AppHash     string
	Phone       string
	SessionFile string
}

// Authenticate выполняет авторизацию в Telegram
func Authenticate(ctx context.Context, config AuthConfig) error {
	// Convert to absolute path if necessary
	sessionFile := config.SessionFile
	if !filepath.IsAbs(sessionFile) {
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		sessionFile = filepath.Join(currentDir, sessionFile)
	}

	fmt.Printf("Using session file: %s\n", sessionFile)

	// Create client
	client := telegram.NewClient(config.AppID, config.AppHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: sessionFile,
		},
	})

	// Use a channel to return errors from the goroutine
	errCh := make(chan error, 1)

	// Run client in a separate goroutine
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Setup auth flow with CodeOnly helper
			flow := auth.NewFlow(
				auth.CodeOnly(config.Phone, &telegramCodeAuth{}),
				auth.SendCodeOptions{},
			)

			// Try to authorize
			fmt.Println("Authorizing...")
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				return fmt.Errorf("authentication error: %w", err)
			}

			// Check successful authorization
			status, err := client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get auth status: %w", err)
			}

			if !status.Authorized {
				return fmt.Errorf("failed to authorize. Check credentials and try again")
			}

			fmt.Println("Successfully authenticated!")
			fmt.Printf("Session saved to: %s\n", sessionFile)
			fmt.Println("Session file saved. Done!")

			return nil
		})

		errCh <- err
	}()

	// Wait for either context done or error from the goroutine
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
