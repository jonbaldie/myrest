package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestForProcessReadsTheConfigFileTheArgumentNames(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = "shop"
db-anon-role = "myrest_anon"
`)

	settings, err := config.ForProcess([]string{path}, config.Environment{})
	if err != nil {
		t.Fatalf("ForProcess: %v", err)
	}
	if settings.DB.AnonRole != "myrest_anon" {
		t.Fatalf("DB.AnonRole = %q, want myrest_anon", settings.DB.AnonRole)
	}
}

func TestForProcessReadsTheEnvironmentWhenThereIsNoArgument(t *testing.T) {
	t.Parallel()

	settings, err := config.ForProcess(nil, config.Environment{
		"MYREST_DB_URI":       "mysql://authenticator@127.0.0.1:3306/",
		"MYREST_DB_SCHEMAS":   "shop",
		"MYREST_DB_ANON_ROLE": "myrest_anon",
	})
	if err != nil {
		t.Fatalf("ForProcess: %v", err)
	}
	if settings.DB.URI != "mysql://authenticator@127.0.0.1:3306/" {
		t.Fatalf("DB.URI = %q, want the MYREST_DB_URI value", settings.DB.URI)
	}
}

func TestForProcessRefusesMoreThanOneArgument(t *testing.T) {
	t.Parallel()

	_, err := config.ForProcess([]string{"first.conf", "second.conf"}, config.Environment{})
	if err == nil {
		t.Fatal("ForProcess accepted two arguments")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error %q does not show the usage of myrest", err)
	}
}

func TestForProcessSaysWhenItCannotReadTheConfigFile(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "absent.conf")

	_, err := config.ForProcess([]string{missing}, config.Environment{})
	if err == nil {
		t.Fatal("ForProcess accepted a config file that is not there")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error %q does not name the config file", err)
	}
}

func TestForProcessRefusesAnIncompleteMinimumRunSet(t *testing.T) {
	t.Parallel()

	_, err := config.ForProcess(nil, config.Environment{"MYREST_DB_ANON_ROLE": "myrest_anon"})

	var incomplete *config.IncompleteRunSetError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error %v is not an *IncompleteRunSetError", err)
	}
}

func TestForProcessRefusesAKnobValueItCannotRead(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "db-max-rows = many\n")

	if _, err := config.ForProcess([]string{path}, config.Environment{}); err == nil {
		t.Fatal("ForProcess accepted db-max-rows = many")
	}
}

// writeConfigFile writes config file text and gives back its path.
func writeConfigFile(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "myrest.conf")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
