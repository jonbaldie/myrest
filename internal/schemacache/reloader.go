package schemacache

import "context"

// Source reads catalog facts for the configured databases.
type Source interface {
	Catalog(context.Context, []string) (Catalog, error)
}

// Reloader reads a fresh catalog and replaces its cache only after a successful read.
type Reloader struct {
	Source    Source
	Databases []string
	Cache     *Cache
}

// Reload replaces the cache with catalog facts for Databases.
func (r Reloader) Reload(ctx context.Context) error {
	catalog, err := r.Source.Catalog(ctx, r.Databases)
	if err != nil {
		return err
	}
	r.Cache.Replace(catalog)
	return nil
}
