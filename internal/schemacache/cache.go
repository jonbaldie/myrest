// Package schemacache holds the in-memory model of the database objects
// myrest serves: which tables the configured MySQL databases hold, and which
// database roles hold the SELECT privilege on them.
package schemacache

// Role is a MySQL account or role that myrest activates for a request.
type Role string

// TableID names one table of one MySQL database. A MySQL database is what the
// db-schemas knob lists, and what MySQL itself calls a schema.
type TableID struct {
	Database string
	Name     string
}

// Column is one column of a table, in catalog order.
type Column struct {
	Name string
}

// Table is a table of a configured MySQL database.
type Table struct {
	ID      TableID
	Columns []Column
}

// ColumnFact says that a table holds a column. The order of the column facts
// of one table is the order the columns keep in the cache.
type ColumnFact struct {
	Table TableID
	Name  string
}

// SelectFact says that a database role holds the SELECT privilege on a table.
type SelectFact struct {
	Role  Role
	Table TableID
}

// Catalog is the catalog data a cache is built from.
type Catalog struct {
	Tables  []TableID
	Columns []ColumnFact
	Selects []SelectFact
}

// Cache answers which table a request can read as a given database role.
type Cache struct {
	tables  map[TableID]Table
	byName  map[string][]TableID
	selects map[Role]map[TableID]bool
}

// Build makes a cache from catalog data.
func Build(catalog Catalog) *Cache {
	cache := &Cache{
		tables:  make(map[TableID]Table, len(catalog.Tables)),
		byName:  make(map[string][]TableID),
		selects: make(map[Role]map[TableID]bool),
	}

	// A column fact or a grant that names no table of the catalog reaches no
	// table here, because only the table facts make a table of the cache.
	columns := make(map[TableID][]Column)
	for _, fact := range catalog.Columns {
		columns[fact.Table] = append(columns[fact.Table], Column{Name: fact.Name})
	}
	for _, id := range catalog.Tables {
		cache.tables[id] = Table{ID: id, Columns: columns[id]}
		cache.byName[id.Name] = append(cache.byName[id.Name], id)
	}
	for _, fact := range catalog.Selects {
		if cache.selects[fact.Role] == nil {
			cache.selects[fact.Role] = make(map[TableID]bool)
		}
		cache.selects[fact.Role][fact.Table] = true
	}
	return cache
}

// Resource gives the table that the name asks for when the database role holds
// the SELECT privilege on it. A table the role cannot select from is not a
// resource, and neither is a table the cache does not hold.
func (c *Cache) Resource(role Role, name string) (Table, bool) {
	for _, id := range c.byName[name] {
		if c.selects[role][id] {
			return c.tables[id], true
		}
	}
	return Table{}, false
}
