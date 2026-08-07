package config_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestDefaultDatabaseIsTheFirstDatabaseOfDbSchemas(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{Schemas: []string{"shop", "warehouse"}},
	}

	if database := settings.DefaultDatabase(); database != "shop" {
		t.Fatalf("DefaultDatabase = %q, want shop", database)
	}
}

func TestDefaultDatabaseIsEmptyWithoutDbSchemas(t *testing.T) {
	t.Parallel()

	if database := (config.Settings{}).DefaultDatabase(); database != "" {
		t.Fatalf("DefaultDatabase = %q, want an empty name", database)
	}
}

func TestHasDatabaseReportsMembershipOfDbSchemas(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{Schemas: []string{"shop", "warehouse"}},
	}
	if !settings.HasDatabase("warehouse") {
		t.Fatal("HasDatabase(warehouse) = false, want true")
	}
	if settings.HasDatabase("tenant3") {
		t.Fatal("HasDatabase(tenant3) = true, want false")
	}
}
