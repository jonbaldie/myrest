// Package schemacache holds the in-memory model of the database objects
// myrest serves: which tables the configured MySQL databases hold, and which
// database roles hold the SELECT privilege on them.
package schemacache

import "strings"

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

// RoleFact says that MySQL granted one role to another, so that the holder
// reads with the privileges of the granted role as well as its own.
type RoleFact struct {
	Holder  Role
	Granted Role
}

// Catalog is the catalog data a cache is built from.
type Catalog struct {
	Tables  []TableID
	Columns []ColumnFact
	Selects []SelectFact
	Roles   []RoleFact
}

// Cache answers which table a request can read as a given database role.
type Cache struct {
	tables  map[TableID]Table
	selects map[Role]map[TableID]bool
}

// Build makes a cache from catalog data.
func Build(catalog Catalog) *Cache {
	cache := &Cache{
		tables:  make(map[TableID]Table, len(catalog.Tables)),
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
	}

	// A role reads with the privileges of the roles granted to it, and of
	// the roles granted to those, as deep as the grants go.
	grants := grantGraph(catalog)
	for role := range grants.roles() {
		cache.selects[role] = grants.reachable(role)
	}
	return cache
}

// grants holds the SELECT privileges of each role and the role grants between
// them, which is what the walk over the grants of one role needs.
type grants struct {
	direct  map[Role]map[TableID]struct{}
	granted map[Role][]Role
}

// grantGraph reads the grant facts of the catalog, by role name.
func grantGraph(catalog Catalog) grants {
	graph := grants{
		direct:  make(map[Role]map[TableID]struct{}),
		granted: make(map[Role][]Role),
	}
	for _, fact := range catalog.Selects {
		role := bareName(fact.Role)
		if graph.direct[role] == nil {
			graph.direct[role] = make(map[TableID]struct{})
		}
		graph.direct[role][fact.Table] = struct{}{}
	}
	for _, fact := range catalog.Roles {
		holder := bareName(fact.Holder)
		graph.granted[holder] = append(graph.granted[holder], bareName(fact.Granted))
	}
	return graph
}

// roles are the roles the cache answers for: a role that holds a grant of its
// own, and a role that holds another role. A role that holds neither reaches
// no table, whether the cache knows the name or not.
func (g grants) roles() map[Role]struct{} {
	found := make(map[Role]struct{})
	for role := range g.direct {
		found[role] = struct{}{}
	}
	for role := range g.granted {
		found[role] = struct{}{}
	}
	return found
}

// reachable gives every table the role can select from: its own grants and the
// grants of every role it reaches.
func (g grants) reachable(role Role) map[TableID]bool {
	found := make(map[TableID]bool)
	g.walk(role, make(map[Role]bool), found)
	return found
}

// walk follows the role grants. A role myrest already walked ends the walk
// there, because MySQL takes a role that is granted back to its holder.
func (g grants) walk(role Role, walked map[Role]bool, found map[TableID]bool) {
	if walked[role] {
		return
	}
	walked[role] = true

	for table := range g.direct[role] {
		found[table] = true
	}
	for _, next := range g.granted[role] {
		g.walk(next, walked, found)
	}
}

// bareName gives the name part of a role. MySQL names a role name@host, and
// the cache keys on the name, so that db-anon-role can carry either shape.
func bareName(role Role) Role {
	name, _, hasHost := strings.Cut(string(role), "@")
	if hasHost {
		return Role(name)
	}
	return role
}

// Resource gives the table the request asks for when the database role holds
// the SELECT privilege on it. A table the role cannot select from is not a
// resource, and neither is a table the cache does not hold. The caller names
// the database, so that one table name can never answer from another one.
func (c *Cache) Resource(role Role, id TableID) (Table, bool) {
	table, held := c.tables[id]
	if !held || !c.selects[bareName(role)][id] {
		return Table{}, false
	}
	return table, true
}
