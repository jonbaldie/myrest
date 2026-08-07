package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	cache := schemacache.Build(catalog)

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     config.ListenAddress(env),
		Settings: settings,
		Cache:    cache,
		Reader:   pool,
		Caller:   pool,
	})
	if err != nil {
		log.Fatalf("myrest: listen: %v", err)
	}
	log.Printf(
		"myrest listening on %s (databases=%s)",
		service.URL(), strings.Join(settings.DB.Schemas, ","),
	)

	go reloadOnSignal(pool, settings.DB.Schemas, cache)

	if err := service.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("myrest: serve: %v", err)
	}
}

// reloadOnSignal is the explicit schema-cache reload path. SIGUSR1 reloads the
// cache from the live catalog. There is no Postgres NOTIFY bus, and config
// changes still need a process restart.
func reloadOnSignal(pool *mysqldb.Pool, databases []string, cache *schemacache.Cache) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	for range signals {
		catalog, err := pool.Catalog(context.Background(), databases)
		if err != nil {
			log.Printf("myrest: reload the schema cache: %v", err)
			continue
		}
		cache.Replace(catalog)
		log.Printf("myrest: reloaded the schema cache")
	}
}
