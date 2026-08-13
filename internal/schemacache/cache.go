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
	Name          string
	DataType      string
	Collation     string
	Nullable      bool
	Default       *string
	Comment       string
	Generated     bool
	AutoIncrement bool
}

// Table is a table of a configured MySQL database.
type Table struct {
	ID      TableID
	Columns []Column
}

// ColumnFact says that a table holds a column. The order of the column facts
// of one table is the order the columns keep in the cache.
type ColumnFact struct {
	Table         TableID
	Name          string
	DataType      string
	Collation     string
	Nullable      bool
	Default       *string
	Comment       string
	Generated     bool
	AutoIncrement bool
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
	Tables []TableID
	Views  []TableID
	// UpdatableViews are the views MySQL marks IS_UPDATABLE = YES. A write
	// through any other view is refused.
	UpdatableViews    []TableID
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
// Replace puts a new catalog into the cache under the lock, so a request that
// reads during a reload still sees one complete snapshot.
type Cache struct {
	mu                sync.RWMutex
	tables            map[TableID]Table
	views             []TableID
	viewSet           map[TableID]bool
	updatableViews    map[TableID]bool
	comments          map[TableID]string
	columns           map[TableID][]Column
	keys              map[TableID][]KeyFact
	foreignKeys       []ForeignKeyFact
	routines          []RoutineFact
	routinesByID      map[RoutineID]RoutineFact
	tablePrivileges   map[Role]map[tablePrivilege]bool
	routinePrivileges map[Role]map[routinePrivilege]bool
}

// Snapshot is one complete, immutable set of schema-cache facts. A request
// can use it while an explicit reload replaces the cache with new facts.
type Snapshot struct {
	tables            map[TableID]Table
	viewSet           map[TableID]bool
	updatableViews    map[TableID]bool
	routines          []RoutineFact
	routinesByID      map[RoutineID]RoutineFact
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

// ReadSnapshot gives one complete schema-cache state. Cache replacement
// installs fresh maps and slices, so these facts stay unchanged after reload.
func ReadSnapshot(c *Cache) Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Snapshot{
		tables:            c.tables,
		viewSet:           c.viewSet,
		updatableViews:    c.updatableViews,
		routines:          c.routines,
		routinesByID:      c.routinesByID,
		tablePrivileges:   c.tablePrivileges,
		routinePrivileges: c.routinePrivileges,
	}
}

// replaceUnlocked builds the maps of the cache from catalog data. The caller
// holds the write lock when the cache already serves requests.
func (c *Cache) replaceUnlocked(catalog Catalog) {
	tables := make(map[TableID]Table, len(catalog.Tables))
	columns := make(map[TableID][]Column)
	comments := make(map[TableID]string, len(catalog.RelationComments))
	keys := make(map[TableID][]KeyFact)

	for _, fact := range catalog.Columns {
		columns[fact.Table] = append(columns[fact.Table], Column{
			Name:          fact.Name,
			DataType:      fact.DataType,
			Collation:     fact.Collation,
			Nullable:      fact.Nullable,
			Default:       fact.Default,
			Comment:       fact.Comment,
			Generated:     fact.Generated,
			AutoIncrement: fact.AutoIncrement,
		})
	}
	for _, id := range catalog.Tables {
		tables[id] = Table{ID: id, Columns: columns[id]}
	}
	// A view is a resource under the same exposure rule as a table: it holds
	// columns and answers Resource when the role has the matching grant.
	for _, id := range catalog.Views {
		tables[id] = Table{ID: id, Columns: columns[id]}
	}
	for _, fact := range catalog.RelationComments {
		comments[fact.Relation] = fact.Comment
	}
	for _, fact := range catalog.Keys {
		keys[fact.Table] = append(keys[fact.Table], fact)
	}

	tablePrivileges := privilegeGraph(catalog)
	routinePrivileges := routinePrivilegeGraph(catalog)

	views, viewSet, updatableViews := indexViews(catalog)
	foreignKeys := append([]ForeignKeyFact(nil), catalog.ForeignKeys...)
	routines, routinesByID := indexRoutines(catalog.Routines)

	c.tables = tables
	c.views = views
	c.viewSet = viewSet
	c.updatableViews = updatableViews
	c.comments = comments
	c.columns = columns
	c.keys = keys
	c.foreignKeys = foreignKeys
	c.routines = routines
	c.routinesByID = routinesByID
	c.tablePrivileges = tablePrivileges
	c.routinePrivileges = routinePrivileges
}

// indexViews copies view ids and builds the view and updatable-view sets.
func indexViews(catalog Catalog) ([]TableID, map[TableID]bool, map[TableID]bool) {
	views := append([]TableID(nil), catalog.Views...)
	viewSet := make(map[TableID]bool, len(catalog.Views))
	for _, id := range catalog.Views {
		viewSet[id] = true
	}
	updatableViews := make(map[TableID]bool, len(catalog.UpdatableViews))
	for _, id := range catalog.UpdatableViews {
		updatableViews[id] = true
	}
	return views, viewSet, updatableViews
}

// indexRoutines copies routine facts and indexes them by id.
func indexRoutines(facts []RoutineFact) ([]RoutineFact, map[RoutineID]RoutineFact) {
	routines := make([]RoutineFact, len(facts))
	routinesByID := make(map[RoutineID]RoutineFact, len(facts))
	for i, fact := range facts {
		copyFact := fact
		copyFact.Parameters = append([]ParameterFact(nil), fact.Parameters...)
		routines[i] = copyFact
		routinesByID[fact.ID] = copyFact
	}
	return routines, routinesByID
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
	for _, fact := range catalog.Selects {
		role := bareName(fact.Role)
		if direct[role] == nil {
			direct[role] = make(map[tablePrivilege]struct{})
		}
		direct[role][tablePrivilege{table: fact.Table, privilege: "SELECT"}] = struct{}{}
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

// grants holds the role grants between database roles.
type grants struct {
	granted map[Role][]Role
}

// grantGraph reads the grant facts of the catalog, by role name.
func grantGraph(catalog Catalog) grants {
	graph := grants{
		granted: make(map[Role][]Role),
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
	for role := range g.granted {
		found[role] = struct{}{}
	}
	return found
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

// Resource gives the table or view the request asks for when the database role
// holds the SELECT privilege on it. A relation the role cannot select from is
// not a resource, and neither is a name the cache does not hold. The caller
// names the database, so that one name can never answer from another one.
func (c *Cache) Resource(role Role, id TableID) (Table, bool) {
	return TableWithPrivilegeFrom(ReadSnapshot(c), role, id, "SELECT")
}

// TableWithPrivilege gives the table or view when the database role holds the
// named privilege on it. Exposure of a resource does not imply every HTTP
// method: INSERT, UPDATE, and DELETE each need their own grant.
func (c *Cache) TableWithPrivilege(role Role, id TableID, privilege string) (Table, bool) {
	return TableWithPrivilegeFrom(ReadSnapshot(c), role, id, privilege)
}

// TableWithPrivilegeFrom gives a table or view of one schema-cache snapshot
// when the database role holds the named privilege on it.
func TableWithPrivilegeFrom(snapshot Snapshot, role Role, id TableID, privilege string) (Table, bool) {
	table, held := snapshot.tables[id]
	if !held || !snapshot.tablePrivileges[bareName(role)][tablePrivilege{table: id, privilege: privilege}] {
		return Table{}, false
	}
	return table, true
}

// TableIDs lists every table and view the cache holds, in no special order.
func TableIDs(c *Cache) []TableID {
	return TableIDsFrom(ReadSnapshot(c))
}

// TableIDsFrom lists every table and view of one schema-cache snapshot.
func TableIDsFrom(snapshot Snapshot) []TableID {
	ids := make([]TableID, 0, len(snapshot.tables))
	for id := range snapshot.tables {
		ids = append(ids, id)
	}
	return ids
}

// HasTable says whether the cache holds this table or view id as a relation.
func (c *Cache) HasTable(id TableID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, held := c.tables[id]
	return held
}

// Views are the views the catalog holds. A view with the matching grant is a
// resource under the same exposure rule as a table.
func (c *Cache) Views() []TableID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]TableID(nil), c.views...)
}

// IsWritable says whether a write may target this relation. A base table is
// writable. A view is writable only when MySQL marks it IS_UPDATABLE = YES.
// Grants still decide which write method is allowed.
func (c *Cache) IsWritable(id TableID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, held := c.tables[id]; !held {
		return false
	}
	if c.viewSet[id] {
		return c.updatableViews[id]
	}
	return true
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
	return RoutinesFrom(ReadSnapshot(c))
}

// RoutinesFrom lists the functions and procedures of one schema-cache snapshot.
func RoutinesFrom(snapshot Snapshot) []RoutineFact {
	return append([]RoutineFact(nil), snapshot.routines...)
}

// Routine is the routine resource of the given name for the active database
// role. A routine is a resource only when the role holds EXECUTE on it, of
// itself or through a role grant.
func (c *Cache) Routine(role Role, id RoutineID) (RoutineFact, bool) {
	return RoutineFrom(ReadSnapshot(c), role, id)
}

// RoutineFrom gives the routine Resource of one schema-cache snapshot for the
// active database role.
func RoutineFrom(snapshot Snapshot, role Role, id RoutineID) (RoutineFact, bool) {
	routine, held := snapshot.routinesByID[id]
	if !held || !snapshot.routinePrivileges[bareName(role)][routinePrivilege{routine: id, privilege: "EXECUTE"}] {
		return RoutineFact{}, false
	}
	routine.Parameters = append([]ParameterFact(nil), routine.Parameters...)
	return routine, true
}

// HasTablePrivilege says whether the role holds a table privilege the exposure
// rule needs, of itself or through a role grant.
func (c *Cache) HasTablePrivilege(role Role, id TableID, privilege string) bool {
	return HasTablePrivilegeFrom(ReadSnapshot(c), role, id, privilege)
}

// HasTableIn says whether the snapshot holds this table or view id as a relation.
func HasTableIn(snapshot Snapshot, id TableID) bool {
	_, held := snapshot.tables[id]
	return held
}

// IsWritableIn says whether a write may target this relation in the snapshot.
func IsWritableIn(snapshot Snapshot, id TableID) bool {
	if _, held := snapshot.tables[id]; !held {
		return false
	}
	if snapshot.viewSet[id] {
		return snapshot.updatableViews[id]
	}
	return true
}

// HasTablePrivilegeFrom says whether the role holds a table privilege in the
// snapshot, of itself or through a role grant.
func HasTablePrivilegeFrom(snapshot Snapshot, role Role, id TableID, privilege string) bool {
	return snapshot.tablePrivileges[bareName(role)][tablePrivilege{table: id, privilege: privilege}]
}

// HasRoutinePrivilege says whether the role holds EXECUTE on a routine, of
// itself or through a role grant.
func (c *Cache) HasRoutinePrivilege(role Role, id RoutineID, privilege string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.routinePrivileges[bareName(role)][routinePrivilege{routine: id, privilege: privilege}]
}
