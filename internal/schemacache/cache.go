// Package schemacache holds the in-memory model of the database objects
// myrest serves: tables, views, columns, keys, foreign keys, routines,
// comments, and the grant data the exposure rule needs.
package schemacache

import (
	"strings"
	"sync"
)

// Role is a MySQL account or role that myrest activates for a request.
type Role string

// TableID names one table of one MySQL database. A MySQL database is what the
// db-schemas knob lists, and what MySQL itself calls a schema.
type TableID struct {
	Database string
	Name     string
}

// Column is one column of a table, in catalog order. A generated column is an
// ordinary column here: select and filter treat it like any other column.
type Column struct {
	Name      string
	DataType  string
	Nullable  bool
	Default   *string
	Comment   string
	Generated bool
}

// Table is a table of a configured MySQL database.
type Table struct {
	ID      TableID
	Columns []Column
}

// ColumnFact says that a table holds a column. The order of the column facts
// of one table is the order the columns keep in the cache.
type ColumnFact struct {
	Table     TableID
	Name      string
	DataType  string
	Nullable  bool
	Default   *string
	Comment   string
	Generated bool
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
	Tables            []TableID
	Views             []TableID
	RelationComments  []CommentFact
	Columns           []ColumnFact
	Keys              []KeyFact
	ForeignKeys       []ForeignKeyFact
	Routines          []RoutineFact
	Selects           []SelectFact
	TablePrivileges   []TablePrivilegeFact
	RoutinePrivileges []RoutinePrivilegeFact
	Roles             []RoleFact
}

// Cache answers which table a request can read as a given database role.
// ReloadSchema replaces the held catalog under the lock, so a request that
// reads during a reload still sees one complete snapshot.
type Cache struct {
	mu                sync.RWMutex
	tables            map[TableID]Table
	views             []TableID
	comments          map[TableID]string
	columns           map[TableID][]Column
	keys              map[TableID][]KeyFact
	foreignKeys       []ForeignKeyFact
	routines          []RoutineFact
	selects           map[Role]map[TableID]bool
	tablePrivileges   map[Role]map[tablePrivilege]bool
	routinePrivileges map[Role]map[routinePrivilege]bool
}

type tablePrivilege struct {
	table     TableID
	privilege string
}

type routinePrivilege struct {
	routine   RoutineID
	privilege string
}

// Build makes a cache from catalog data.
func Build(catalog Catalog) *Cache {
	cache := &Cache{}
	cache.replaceUnlocked(catalog)
	return cache
}

// Replace puts a new catalog into the cache. After it returns, Resource answers
// from the new facts alone.
func (c *Cache) Replace(catalog Catalog) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replaceUnlocked(catalog)
}

// replaceUnlocked builds the maps of the cache from catalog data. The caller
// holds the write lock when the cache already serves requests.
func (c *Cache) replaceUnlocked(catalog Catalog) {
	tables := make(map[TableID]Table, len(catalog.Tables))
	columns := make(map[TableID][]Column)
	comments := make(map[TableID]string, len(catalog.RelationComments))
	keys := make(map[TableID][]KeyFact)
	selects := make(map[Role]map[TableID]bool)

	for _, fact := range catalog.Columns {
		columns[fact.Table] = append(columns[fact.Table], Column{
			Name:      fact.Name,
			DataType:  fact.DataType,
			Nullable:  fact.Nullable,
			Default:   fact.Default,
			Comment:   fact.Comment,
			Generated: fact.Generated,
		})
	}
	for _, id := range catalog.Tables {
		tables[id] = Table{ID: id, Columns: columns[id]}
	}
	for _, fact := range catalog.RelationComments {
		comments[fact.Relation] = fact.Comment
	}
	for _, fact := range catalog.Keys {
		keys[fact.Table] = append(keys[fact.Table], fact)
	}

	grants := grantGraph(catalog)
	for role := range grants.roles() {
		selects[role] = grants.reachable(role)
	}

	tablePrivileges := privilegeGraph(catalog)
	routinePrivileges := routinePrivilegeGraph(catalog)

	views := append([]TableID(nil), catalog.Views...)
	foreignKeys := append([]ForeignKeyFact(nil), catalog.ForeignKeys...)
	routines := append([]RoutineFact(nil), catalog.Routines...)

	c.tables = tables
	c.views = views
	c.comments = comments
	c.columns = columns
	c.keys = keys
	c.foreignKeys = foreignKeys
	c.routines = routines
	c.selects = selects
	c.tablePrivileges = tablePrivileges
	c.routinePrivileges = routinePrivileges
}

// privilegeGraph expands table privileges through the role-grant graph.
func privilegeGraph(catalog Catalog) map[Role]map[tablePrivilege]bool {
	direct := make(map[Role]map[tablePrivilege]struct{})
	for _, fact := range catalog.TablePrivileges {
		role := bareName(fact.Role)
		if direct[role] == nil {
			direct[role] = make(map[tablePrivilege]struct{})
		}
		direct[role][tablePrivilege{table: fact.Table, privilege: fact.Privilege}] = struct{}{}
	}
	grants := grantGraph(catalog)
	found := make(map[Role]map[tablePrivilege]bool)
	for role := range grants.roles() {
		held := make(map[tablePrivilege]bool)
		walkTablePrivileges(role, grants.granted, direct, make(map[Role]bool), held)
		found[role] = held
	}
	for role := range direct {
		if found[role] != nil {
			continue
		}
		held := make(map[tablePrivilege]bool)
		walkTablePrivileges(role, grants.granted, direct, make(map[Role]bool), held)
		found[role] = held
	}
	return found
}

func walkTablePrivileges(
	role Role,
	granted map[Role][]Role,
	direct map[Role]map[tablePrivilege]struct{},
	walked map[Role]bool,
	found map[tablePrivilege]bool,
) {
	if walked[role] {
		return
	}
	walked[role] = true
	for privilege := range direct[role] {
		found[privilege] = true
	}
	for _, next := range granted[role] {
		walkTablePrivileges(next, granted, direct, walked, found)
	}
}

func routinePrivilegeGraph(catalog Catalog) map[Role]map[routinePrivilege]bool {
	direct := make(map[Role]map[routinePrivilege]struct{})
	for _, fact := range catalog.RoutinePrivileges {
		role := bareName(fact.Role)
		if direct[role] == nil {
			direct[role] = make(map[routinePrivilege]struct{})
		}
		direct[role][routinePrivilege{routine: fact.Routine, privilege: fact.Privilege}] = struct{}{}
	}
	grants := grantGraph(catalog)
	found := make(map[Role]map[routinePrivilege]bool)
	for role := range grants.roles() {
		held := make(map[routinePrivilege]bool)
		walkRoutinePrivileges(role, grants.granted, direct, make(map[Role]bool), held)
		found[role] = held
	}
	for role := range direct {
		if found[role] != nil {
			continue
		}
		held := make(map[routinePrivilege]bool)
		walkRoutinePrivileges(role, grants.granted, direct, make(map[Role]bool), held)
		found[role] = held
	}
	return found
}

func walkRoutinePrivileges(
	role Role,
	granted map[Role][]Role,
	direct map[Role]map[routinePrivilege]struct{},
	walked map[Role]bool,
	found map[routinePrivilege]bool,
) {
	if walked[role] {
		return
	}
	walked[role] = true
	for privilege := range direct[role] {
		found[privilege] = true
	}
	for _, next := range granted[role] {
		walkRoutinePrivileges(next, granted, direct, walked, found)
	}
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
	c.mu.RLock()
	defer c.mu.RUnlock()

	table, held := c.tables[id]
	if !held || !c.selects[bareName(role)][id] {
		return Table{}, false
	}
	return table, true
}

// Views are the views the catalog holds for later tickets. This ticket does
// not expose them as HTTP resources.
func (c *Cache) Views() []TableID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]TableID(nil), c.views...)
}

// Comment is the comment MySQL holds on a table or a view.
func (c *Cache) Comment(id TableID) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.comments[id]
}

// ColumnsOf are the columns of a table or view, in catalog order.
func (c *Cache) ColumnsOf(id TableID) []Column {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Column(nil), c.columns[id]...)
}

// KeysOf are the primary and unique keys of a table or view.
func (c *Cache) KeysOf(id TableID) []KeyFact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]KeyFact(nil), c.keys[id]...)
}

// ForeignKeys are the foreign keys the catalog declares.
func (c *Cache) ForeignKeys() []ForeignKeyFact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]ForeignKeyFact(nil), c.foreignKeys...)
}

// Routines are the functions and procedures the catalog holds.
func (c *Cache) Routines() []RoutineFact {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]RoutineFact(nil), c.routines...)
}

// HasTablePrivilege says whether the role holds a table privilege the exposure
// rule needs, of itself or through a role grant.
func (c *Cache) HasTablePrivilege(role Role, id TableID, privilege string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tablePrivileges[bareName(role)][tablePrivilege{table: id, privilege: privilege}]
}

// HasRoutinePrivilege says whether the role holds EXECUTE on a routine, of
// itself or through a role grant.
func (c *Cache) HasRoutinePrivilege(role Role, id RoutineID, privilege string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.routinePrivileges[bareName(role)][routinePrivilege{routine: id, privilege: privilege}]
}
