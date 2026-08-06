// Package schemacache holds the in-memory model of the database objects
// myrest serves: which tables the configured MySQL databases hold, and which
// database roles hold the SELECT privilege on them.
package schemacache

// Column is one column of a table, in catalog order.
type Column struct {
	Name string
}

// Table is a table of a configured MySQL database.
type Table struct {
	Schema  string
	Name    string
	Columns []Column
}

// TableFact says that a configured database holds a table.
type TableFact struct {
	Schema string
	Name   string
}

// ColumnFact says that a table holds a column. The order of the column facts
// of one table is the order the columns keep in the cache.
type ColumnFact struct {
	Schema string
	Table  string
	Name   string
}

// SelectFact says that a database role holds the SELECT privilege on a table.
type SelectFact struct {
	Role   string
	Schema string
	Table  string
}

// Catalog is the catalog data a cache is built from.
type Catalog struct {
	Tables  []TableFact
	Columns []ColumnFact
	Selects []SelectFact
}

// Cache answers which table a request can read as a given database role.
type Cache struct {
	tables  map[string]Table
	byName  map[string][]string
	selects map[string]map[string]bool
}

// Build makes a cache from catalog data. Facts about a table the catalog does
// not hold are left out, so a column or a grant on an unknown table is ignored.
func Build(catalog Catalog) *Cache {
	cache := &Cache{
		tables:  make(map[string]Table, len(catalog.Tables)),
		byName:  make(map[string][]string),
		selects: make(map[string]map[string]bool),
	}
	for _, fact := range catalog.Tables {
		key := tableKey(fact.Schema, fact.Name)
		if _, known := cache.tables[key]; known {
			continue
		}
		cache.tables[key] = Table{Schema: fact.Schema, Name: fact.Name}
		cache.byName[fact.Name] = append(cache.byName[fact.Name], key)
	}
	for _, fact := range catalog.Columns {
		key := tableKey(fact.Schema, fact.Table)
		table, known := cache.tables[key]
		if !known {
			continue
		}
		table.Columns = append(table.Columns, Column{Name: fact.Name})
		cache.tables[key] = table
	}
	for _, fact := range catalog.Selects {
		key := tableKey(fact.Schema, fact.Table)
		if _, known := cache.tables[key]; !known {
			continue
		}
		if cache.selects[fact.Role] == nil {
			cache.selects[fact.Role] = make(map[string]bool)
		}
		cache.selects[fact.Role][key] = true
	}
	return cache
}

// Resource gives the table that the name asks for when the database role holds
// the SELECT privilege on it. A table the role cannot select from is not a
// resource, and neither is a table the cache does not hold.
func (c *Cache) Resource(role, name string) (Table, bool) {
	for _, key := range c.byName[name] {
		if c.selects[role][key] {
			return c.tables[key], true
		}
	}
	return Table{}, false
}

func tableKey(schema, table string) string {
	return schema + "." + table
}
