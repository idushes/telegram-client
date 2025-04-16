package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Create a context that we can cancel on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Run the application in a separate goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- runApp(ctx)
	}()

	// Wait for either the app to finish or an interrupt signal
	select {
	case err := <-errCh:
		if err != nil {
			log.Printf("Application error: %v", err)
			os.Exit(1)
		}
	case sig := <-signalCh:
		log.Printf("Received signal: %v", sig)
		log.Println("Shutting down gracefully...")

		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		// Cancel the main context to signal all operations to stop
		cancel()

		// Wait for graceful shutdown or timeout
		select {
		case err := <-errCh:
			if err != nil {
				log.Printf("Error during shutdown: %v", err)
			}
		case <-shutdownCtx.Done():
			log.Println("Shutdown timed out")
		}
	}

	log.Println("Application stopped")
}

// runApp is the main application function
func runApp(ctx context.Context) error {
	log.Println("Application starting...")

	// Your application initialization here

	// Simulating a long-running process
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, cleaning up...")
			// Perform any cleanup operations here
			time.Sleep(1 * time.Second) // Simulate cleanup work
			return nil
		case t := <-ticker.C:
			fmt.Printf("Working... %s\n", t.Format(time.RFC3339))
		}
	}
}
