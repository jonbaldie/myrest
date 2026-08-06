package config_test

import (
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestLoadReadsTheMinimumRunSetFromEnvironmentVariables(t *testing.T) {
	t.Parallel()

	settings, err := config.Load("", config.Environment{
		"MYREST_DB_URI":       "mysql://authenticator@127.0.0.1:3306/",
		"MYREST_DB_SCHEMAS":   "myrest_fixture",
		"MYREST_DB_ANON_ROLE": "myrest_anon",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if settings.DB.URI != "mysql://authenticator@127.0.0.1:3306/" {
		t.Errorf("DBURI = %q, want the MYREST_DB_URI value", settings.DB.URI)
	}
	if !reflect.DeepEqual(settings.DB.Schemas, []string{"myrest_fixture"}) {
		t.Errorf("DBSchemas = %v, want [myrest_fixture]", settings.DB.Schemas)
	}
	if settings.DB.AnonRole != "myrest_anon" {
		t.Errorf("DBAnonRole = %q, want myrest_anon", settings.DB.AnonRole)
	}
}

// cfg-002: a config file and the MYREST_* variables must give the same result.
func TestLoadGivesTheSameSettingsFromAFileAsFromTheEnvironment(t *testing.T) {
	t.Parallel()

	file := `# myrest.conf
db-uri = "mysql://authenticator@127.0.0.1:3306/"

db-schemas = "myrest_fixture"
db-anon-role = "myrest_anon"
`
	fromFile, err := config.Load(file, config.Environment{})
	if err != nil {
		t.Fatalf("Load from file: %v", err)
	}

	fromEnvironment, err := config.Load("", config.Environment{
		"MYREST_DB_URI":       "mysql://authenticator@127.0.0.1:3306/",
		"MYREST_DB_SCHEMAS":   "myrest_fixture",
		"MYREST_DB_ANON_ROLE": "myrest_anon",
	})
	if err != nil {
		t.Fatalf("Load from environment: %v", err)
	}

	if !reflect.DeepEqual(fromFile, fromEnvironment) {
		t.Fatalf("file settings %+v differ from environment settings %+v", fromFile, fromEnvironment)
	}
}

func TestLoadLetsAnEnvironmentVariableWinOverTheSameKnobInTheFile(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(
		`db-anon-role = "from_file"`,
		config.Environment{"MYREST_DB_ANON_ROLE": "from_environment"},
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if settings.DB.AnonRole != "from_environment" {
		t.Fatalf("DBAnonRole = %q, want from_environment", settings.DB.AnonRole)
	}
}

func TestLoadAcceptsMoreThanOneMySQLDatabaseInDBSchemas(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(`db-schemas = "shop, warehouse ,billing"`, config.Environment{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := []string{"shop", "warehouse", "billing"}
	if !reflect.DeepEqual(settings.DB.Schemas, want) {
		t.Fatalf("DBSchemas = %v, want %v", settings.DB.Schemas, want)
	}
}

func TestEnvironmentFromKeepsAValueThatHoldsAnEqualsSign(t *testing.T) {
	t.Parallel()

	env := config.EnvironmentFrom([]string{
		"MYREST_DB_URI=mysql://authenticator@127.0.0.1:3306/?tls=true",
		"PATH=/usr/bin",
	})

	want := config.Environment{
		"MYREST_DB_URI": "mysql://authenticator@127.0.0.1:3306/?tls=true",
		"PATH":          "/usr/bin",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("EnvironmentFrom = %v, want %v", env, want)
	}
}
