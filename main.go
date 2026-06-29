package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env if it exists
	_ = godotenv.Load()

	syncDir := flag.String("sync", "", "Directory to sync files to")
	serverURL := flag.String("url", "", "Server URL for syncing (e.g., https://drop.example.com)")
	token := flag.String("token", "", "Authentication token for syncing")
	port := flag.String("port", "8080", "Port to run the server on (default 8080)")
	flag.Parse()

	// If --sync is provided, run as client
	if *syncDir != "" {
		if *serverURL == "" {
			log.Fatal("--url is required when running in sync mode")
		}
		
		// If token isn't provided via flag, try env
		if *token == "" {
			*token = os.Getenv("AUTH_TOKEN")
		}

		client := NewClient(*serverURL, *token, *syncDir)
		err := client.Sync()
		if err != nil {
			log.Fatalf("Sync failed: %v", err)
		}
		return
	}

	// Otherwise, run as server
	ctx := context.Background()
	r2Client, err := NewR2Client(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize R2 client: %v", err)
	}

	server := NewServer(r2Client)
	handler := server.SetupRoutes()

	addr := ":" + *port
	// Cloud Run sets PORT environment variable
	if envPort := os.Getenv("PORT"); envPort != "" {
		addr = ":" + envPort
	}

	log.Printf("Starting server on %s", addr)
	err = http.ListenAndServe(addr, handler)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
