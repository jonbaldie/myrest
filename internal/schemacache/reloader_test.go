package schemacache_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestReloadReadsExactlyTheConfiguredDatabases(t *testing.T) {
	configured := []string{"inventory", "shop"}
	source := &recordingCatalogSource{}
	reloader := schemacache.Reloader{
		Source:    source,
		Databases: configured,
		Cache:     schemacache.Build(schemacache.Catalog{}),
	}

	if err := reloader.Reload(context.Background()); err != nil {
		t.Fatalf("Reload error = %v, want nil", err)
	}
	if !reflect.DeepEqual(source.databases, configured) {
		t.Fatalf("catalog databases = %#v, want %#v", source.databases, configured)
	}
}

type recordingCatalogSource struct {
	databases []string
}

func (source *recordingCatalogSource) Catalog(_ context.Context, databases []string) (schemacache.Catalog, error) {
	source.databases = append([]string(nil), databases...)
	return schemacache.Catalog{}, nil
}

func TestReloadKeepsTheOldCacheWhenCatalogReadFails(t *testing.T) {
	readErr := errors.New("catalog unavailable")
	resource := schemacache.TableID{Database: "shop", Name: "items"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{resource},
		Columns: []schemacache.ColumnFact{{Table: resource, Name: "old_name"}},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: resource}},
	})
	reloader := schemacache.Reloader{
		Source: failingCatalogSource{err: readErr},
		Cache:  cache,
	}

	if err := reloader.Reload(context.Background()); !errors.Is(err, readErr) {
		t.Fatalf("Reload error = %v, want %v", err, readErr)
	}

	found, ok := cache.Resource(anonRole, resource)
	if !ok {
		t.Fatal("shop.items stopped being a resource after the catalog read failed")
	}
	if want := []schemacache.Column{{Name: "old_name"}}; !reflect.DeepEqual(found.Columns, want) {
		t.Fatalf("columns after failed reload = %#v, want %#v", found.Columns, want)
	}
}

type failingCatalogSource struct {
	err error
}

func (source failingCatalogSource) Catalog(context.Context, []string) (schemacache.Catalog, error) {
	return schemacache.Catalog{}, source.err
}
