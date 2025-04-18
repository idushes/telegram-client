package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	// Парсим конфигурацию
	config, err := ParseConfig()
	if err != nil {
		fmt.Println("Error:", err)
		PrintUsage()
		os.Exit(1)
	}

	// Create context with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Run authentication
	if err := Authenticate(ctx, config); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		os.Exit(1)
	}
}
