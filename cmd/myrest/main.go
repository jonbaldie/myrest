package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jonbaldie/myrest/internal/httpapi"
)

func main() {
	addr := envOr("MYREST_LISTEN", "127.0.0.1:3000")
	service, err := httpapi.Listen(addr)
	if err != nil {
		log.Fatalf("myrest: listen: %v", err)
	}
	log.Printf("myrest listening on %s (parity=none)", service.URL())
	if err := service.Serve(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("myrest: serve: %v", err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
