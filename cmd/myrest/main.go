package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
)

func main() {
	env := config.EnvironmentFrom(os.Environ())
	settings, err := config.ForProcess(os.Args[1:], env)
	if err != nil {
		log.Fatalf("myrest: %v", err)
	}

	service, err := httpapi.Listen(config.ListenAddress(env))
	if err != nil {
		log.Fatalf("myrest: listen: %v", err)
	}
	log.Printf(
		"myrest listening on %s (databases=%s; parity=none)",
		service.URL(), strings.Join(settings.DB.Schemas, ","),
	)

	if err := service.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("myrest: serve: %v", err)
	}
}
