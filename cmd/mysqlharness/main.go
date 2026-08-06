package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jonbaldie/myrest/internal/mysqltest"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <fixture.sql> [more.sql...]", filepath.Base(os.Args[0]))
	}

	harness, err := mysqltest.Start()
	if err != nil {
		log.Fatalf("start MySQL: %v", err)
	}
	defer func() {
		if stopErr := harness.Stop(); stopErr != nil {
			log.Printf("stop MySQL: %v", stopErr)
		}
	}()

	if err := harness.LoadSQL(os.Args[1:]...); err != nil {
		log.Fatalf("load fixtures: %v", err)
	}

	fmt.Printf("MySQL 8 ready\nDSN=%s\nRootDSN=%s\n", harness.DSN(), harness.RootDSN())
	fmt.Println("Press Ctrl+C to stop.")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}
