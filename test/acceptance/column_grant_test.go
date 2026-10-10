package acceptance_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// createPaySlips makes a table the anonymous role holds no table grant on,
// runs the given grants, and drops the table and the grants after the test.
func createPaySlips(t *testing.T, grants ...string) {
	t.Helper()

	statements := append([]string{
		`CREATE TABLE myrest_fixture.pay_slips (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			employee VARCHAR(255) NOT NULL,
			salary INT NOT NULL DEFAULT 0,
			PRIMARY KEY (id)
		) ENGINE=InnoDB`,
		`INSERT INTO myrest_fixture.pay_slips (employee, salary) VALUES ('ada', 1000)`,
	}, grants...)
	t.Cleanup(func() {
		// DROP TABLE takes away the column grants on the table too.
		_ = harness.Exec("DROP TABLE IF EXISTS myrest_fixture.pay_slips")
		_ = harness.Exec("DROP ROLE IF EXISTS 'pay_reader'")
	})
	for _, statement := range statements {
		if err := harness.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// cache-001 for a column grant: a role that holds SELECT on some columns of a
// table reads those columns.
func TestColumnSelectGrantIsAResource(t *testing.T) {
	createPaySlips(t, "GRANT SELECT (id, employee) ON myrest_fixture.pay_slips TO 'myrest_anon'")
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/pay_slips?select=id,employee")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"employee":"ada"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	// MySQL hides a column from information_schema.COLUMNS when the
	// authenticator holds no privilege on it, so the schema cache does not
	// know the column.
	response, body = get(t, service, "/pay_slips?select=salary")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST204")

	response, body = apitest.Do(t, http.MethodOptions, service.URL()+"/pay_slips", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OPTIONS status = %d; body = %s", response.StatusCode, body)
	}
	if got := response.Header.Get("Allow"); got != "OPTIONS,GET,HEAD" {
		t.Fatalf("Allow = %q, want OPTIONS,GET,HEAD", got)
	}

	response, body = get(t, service, "/")
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode the OpenAPI document: %v; status = %d", err, response.StatusCode)
	}
	operations := document.Paths["/pay_slips"]
	if _, held := operations["get"]; !held || len(operations) != 1 {
		t.Fatalf("OpenAPI /pay_slips operations = %v, want get only", operations)
	}
}

// When another role of the authenticator holds the whole table, the schema
// cache knows every column. MySQL then refuses a column outside the grant of
// the active role (error 1143), and the refusal maps as error 1142 does.
func TestColumnOutsideTheColumnGrantIsForbidden(t *testing.T) {
	createPaySlips(t,
		"GRANT SELECT (id, employee) ON myrest_fixture.pay_slips TO 'myrest_anon'",
		"GRANT SELECT ON myrest_fixture.pay_slips TO 'web-anon'",
	)
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/pay_slips?select=id,employee")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}

	response, body = get(t, service, "/pay_slips?select=salary")
	apitest.AssertEnvelope(t, response, body, http.StatusForbidden, "MYREST002")
}

// Column INSERT and UPDATE grants open POST and PATCH, and MySQL takes the
// write on the granted columns.
func TestColumnWriteGrantsOpenWrites(t *testing.T) {
	createPaySlips(t,
		"GRANT SELECT (id, employee), INSERT (employee), UPDATE (employee) ON myrest_fixture.pay_slips TO 'myrest_anon'",
	)
	service := serve(t, "myrest_fixture")

	response, body := apitest.Do(t, http.MethodOptions, service.URL()+"/pay_slips", nil)
	if got := response.Header.Get("Allow"); got != "OPTIONS,GET,HEAD,POST,PUT,PATCH" {
		t.Fatalf("Allow = %q, want OPTIONS,GET,HEAD,POST,PUT,PATCH; body = %s", got, body)
	}

	response, body = apitest.PostJSON(t, service.URL()+"/pay_slips", `{"employee":"bob"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	response, body = apitest.DoBody(
		t, http.MethodPatch, service.URL()+"/pay_slips?employee=eq.bob",
		http.Header{"Content-Type": {"application/json"}}, `{"employee":"carol"}`,
	)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}

	response, body = get(t, service, "/pay_slips?select=employee&order=id.asc")
	if want := `[{"employee":"ada"},{"employee":"carol"}]`; string(body) != want+"\n" {
		t.Fatalf("status = %d, body = %s, want %s", response.StatusCode, body, want)
	}
}

// A column grant held through a role granted to the active role counts too.
func TestColumnGrantThroughAnotherRoleIsAResource(t *testing.T) {
	createPaySlips(t,
		"CREATE ROLE IF NOT EXISTS 'pay_reader'",
		"GRANT SELECT (id, employee) ON myrest_fixture.pay_slips TO 'pay_reader'",
		"GRANT 'pay_reader' TO 'myrest_anon'",
	)

	response, body := get(t, serve(t, "myrest_fixture"), "/pay_slips?select=id,employee")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
}

// cache-003 for a column grant: after the revoke and a reload, the table is
// not a resource.
func TestRevokedColumnGrantHidesTheTableAfterReload(t *testing.T) {
	createPaySlips(t, "GRANT SELECT (id, employee) ON myrest_fixture.pay_slips TO 'myrest_anon'")
	databases := []string{"myrest_fixture"}
	pool, cache, service := serveWithPool(t, databases...)

	if response, body := get(t, service, "/pay_slips?select=id"); response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d before the revoke; body = %s", response.StatusCode, body)
	}

	if err := harness.Exec("REVOKE SELECT (id, employee) ON myrest_fixture.pay_slips FROM 'myrest_anon'"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	reloader := schemacache.Reloader{Source: pool, Databases: databases, Cache: cache}
	if err := reloader.Reload(t.Context()); err != nil {
		t.Fatalf("reload the schema cache: %v", err)
	}

	response, body := get(t, service, "/pay_slips?select=id")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}
