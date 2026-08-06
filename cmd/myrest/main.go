package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func main() {
	env := config.EnvironmentFrom(os.Environ())
	settings, err := config.ForProcess(os.Args[1:], env)
	if err != nil {
		log.Fatalf("myrest: %v", err)
	}

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		log.Fatalf("myrest: %v", err)
	}
	defer func() { _ = pool.Close() }()

	catalog, err := pool.Catalog(context.Background(), settings.DB.Schemas)
	if err != nil {
		log.Fatalf("myrest: read the catalog: %v", err)
	}

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     config.ListenAddress(env),
		Settings: settings,
		Cache:    schemacache.Build(catalog),
		Reader:   pool,
	})
	if err != nil {
		log.Fatalf("myrest: listen: %v", err)
	}
	log.Printf(
		"myrest listening on %s (databases=%s)",
		service.URL(), strings.Join(settings.DB.Schemas, ","),
	)

	if err := service.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("myrest: serve: %v", err)
	}
}
