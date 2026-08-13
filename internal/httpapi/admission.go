package httpapi

import (
	"net/http"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// requestedResource identifies a requested Resource under the selected
// database role.
type requestedResource struct {
	role     schemacache.Role
	database string
	name     string
}

type resourceNeed int

const (
	_ resourceNeed = iota
	readTableNeed
	writeTableNeed
	routineNeed
	tableMethodsNeed
)

type admissionRequest struct {
	requested requestedResource
	need      resourceNeed
	privilege string
}

type admissionRefusal int

const (
	noAdmissionRefusal admissionRefusal = iota
	noTableRefusal
	nonWritableViewRefusal
	noRoutineRefusal
)

type admissionResult struct {
	table   schemacache.Table
	routine schemacache.RoutineFact
	methods []string
	refusal admissionRefusal
}

// selectResource selects a requested Resource for a known database role. The
// caller chooses the profile header and route name for its HTTP method.
func (s *Service) selectResource(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	profileHeader string,
	name string,
) (requestedResource, bool) {
	database, ok := s.requestDatabase(writer, request, profileHeader)
	if !ok {
		return requestedResource{}, false
	}
	return requestedResource{role: role, database: database, name: name}, true
}

func (requested requestedResource) table() schemacache.TableID {
	return schemacache.TableID{Database: requested.database, Name: requested.name}
}

func (requested requestedResource) routine() schemacache.RoutineID {
	return schemacache.RoutineID{Database: requested.database, Name: requested.name}
}

// admit resolves one HTTP Resource need from one complete schema-cache
// snapshot. Its callers keep only the HTTP-specific refusal and response work.
func (s *Service) admit(request admissionRequest) admissionResult {
	return admitFrom(schemacache.ReadSnapshot(s.cache), request)
}

func admitFrom(snapshot schemacache.Snapshot, request admissionRequest) admissionResult {
	switch request.need {
	case readTableNeed:
		return admitReadTable(snapshot, request.requested)
	case writeTableNeed:
		return admitWriteTable(snapshot, request.requested, request.privilege)
	case routineNeed:
		return admitRoutine(snapshot, request.requested)
	case tableMethodsNeed:
		return admitTableMethods(snapshot, request.requested)
	default:
		panic("unknown Resource admission need")
	}
}

func admitReadTable(snapshot schemacache.Snapshot, requested requestedResource) admissionResult {
	table, ok := schemacache.TableWithPrivilegeFrom(snapshot, requested.role, requested.table(), "SELECT")
	if !ok {
		return admissionResult{refusal: noTableRefusal}
	}
	return admissionResult{table: table}
}

func admitWriteTable(
	snapshot schemacache.Snapshot,
	requested requestedResource,
	privilege string,
) admissionResult {
	id := requested.table()
	table, ok := schemacache.TableWithPrivilegeFrom(snapshot, requested.role, id, privilege)
	if !ok {
		return admissionResult{refusal: noTableRefusal}
	}
	if !schemacache.IsWritableIn(snapshot, id) {
		return admissionResult{refusal: nonWritableViewRefusal}
	}
	return admissionResult{table: table}
}

func admitRoutine(snapshot schemacache.Snapshot, requested requestedResource) admissionResult {
	routine, ok := schemacache.RoutineFrom(snapshot, requested.role, requested.routine())
	if !ok {
		return admissionResult{refusal: noRoutineRefusal}
	}
	return admissionResult{routine: routine}
}

func admitTableMethods(snapshot schemacache.Snapshot, requested requestedResource) admissionResult {
	id := requested.table()
	if !schemacache.HasTableIn(snapshot, id) {
		return admissionResult{refusal: noTableRefusal}
	}
	methods := []string{http.MethodOptions}
	usable := false
	if schemacache.HasTablePrivilegeFrom(snapshot, requested.role, id, "SELECT") {
		methods = append(methods, http.MethodGet, http.MethodHead)
		usable = true
	}
	if schemacache.IsWritableIn(snapshot, id) {
		methods, usable = appendWriteAllowMethods(methods, usable, snapshot, requested.role, id)
	}
	if !usable {
		return admissionResult{refusal: noTableRefusal}
	}
	return admissionResult{methods: methods}
}

// admitReadResource admits a readable table Resource. A refused Resource
// always gets the established table-not-found response.
func (s *Service) admitReadResource(
	writer http.ResponseWriter,
	requested requestedResource,
) (schemacache.Table, bool) {
	asked := requested.table()
	admission := s.admit(admissionRequest{requested: requested, need: readTableNeed})
	if admission.refusal != noAdmissionRefusal {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return schemacache.Table{}, false
	}
	return admission.table, true
}

// admitWriteResource admits a writable table Resource with the method grant.
// A denied grant and a non-updatable view keep their established refusals.
func (s *Service) admitWriteResource(
	writer http.ResponseWriter,
	requested requestedResource,
	privilege string,
) (schemacache.Table, bool) {
	asked := requested.table()
	admission := s.admit(admissionRequest{
		requested: requested,
		need:      writeTableNeed,
		privilege: privilege,
	})
	if admission.refusal == noTableRefusal {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return schemacache.Table{}, false
	}
	if admission.refusal == nonWritableViewRefusal {
		writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, "The view is not updatable")
		return schemacache.Table{}, false
	}
	return admission.table, true
}

// admitRoutineResource admits a routine Resource with EXECUTE. A refused
// Resource always gets the established routine-not-found response.
func (s *Service) admitRoutineResource(
	writer http.ResponseWriter,
	requested requestedResource,
) (schemacache.RoutineFact, bool) {
	asked := requested.routine()
	admission := s.admit(admissionRequest{requested: requested, need: routineNeed})
	if admission.refusal != noAdmissionRefusal {
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return schemacache.RoutineFact{}, false
	}
	return admission.routine, true
}

// tableAllowMethods gives the methods OPTIONS and discovery can advertise for
// one Resource. The snapshot keeps all grant and writability facts together.
func (s *Service) tableAllowMethods(requested requestedResource) []string {
	return s.admit(admissionRequest{requested: requested, need: tableMethodsNeed}).methods
}

func tableAllowMethods(snapshot schemacache.Snapshot, requested requestedResource) []string {
	return admitFrom(snapshot, admissionRequest{requested: requested, need: tableMethodsNeed}).methods
}

func appendWriteAllowMethods(
	methods []string,
	usable bool,
	snapshot schemacache.Snapshot,
	role schemacache.Role,
	id schemacache.TableID,
) ([]string, bool) {
	if schemacache.HasTablePrivilegeFrom(snapshot, role, id, "INSERT") {
		methods = append(methods, http.MethodPost, http.MethodPut)
		usable = true
	}
	if schemacache.HasTablePrivilegeFrom(snapshot, role, id, "UPDATE") {
		methods = append(methods, http.MethodPatch)
		usable = true
	}
	if schemacache.HasTablePrivilegeFrom(snapshot, role, id, "DELETE") {
		methods = append(methods, http.MethodDelete)
		usable = true
	}
	return methods, usable
}
