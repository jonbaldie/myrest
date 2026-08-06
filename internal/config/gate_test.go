package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestGateRefusesAnEmptyMinimumRunSetAndNamesEveryMissingKnob(t *testing.T) {
	t.Parallel()

	err := config.Gate(config.Settings{})
	if err == nil {
		t.Fatal("Gate accepted settings with an empty minimum run set")
	}

	message := err.Error()
	for _, knob := range []string{"db-uri", "db-schemas", "jwt-secret", "db-anon-role"} {
		if !strings.Contains(message, knob) {
			t.Errorf("gate message %q does not name the missing knob %q", message, knob)
		}
	}
}

func TestGateAcceptsJWTSecretAloneAsTheAuthPartOfTheRunSet(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			URI:     "mysql://authenticator@127.0.0.1:3306/",
			Schemas: []string{"myrest_fixture"},
		},
		JWT: config.JWTSettings{Secret: "reallyreallyreallyreallyverysafe"},
	}

	if err := config.Gate(settings); err != nil {
		t.Fatalf("Gate refused a complete run set with jwt-secret only: %v", err)
	}
}

func TestGateAcceptsAnonymousDatabaseRoleAloneAsTheAuthPartOfTheRunSet(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			URI:      "mysql://authenticator@127.0.0.1:3306/",
			Schemas:  []string{"myrest_fixture"},
			AnonRole: "myrest_anon",
		},
	}

	if err := config.Gate(settings); err != nil {
		t.Fatalf("Gate refused a complete run set with db-anon-role only: %v", err)
	}
}

func TestGateNamesOnlyTheKnobThatIsMissing(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			Schemas:  []string{"myrest_fixture"},
			AnonRole: "myrest_anon",
		},
	}

	err := config.Gate(settings)
	if err == nil {
		t.Fatal("Gate accepted settings without db-uri")
	}

	var incomplete *config.IncompleteRunSetError
	if !errors.As(err, &incomplete) {
		t.Fatalf("Gate error %v is not an *IncompleteRunSetError", err)
	}
	if len(incomplete.Missing) != 1 || incomplete.Missing[0] != "db-uri" {
		t.Fatalf("Missing = %v, want [db-uri]", incomplete.Missing)
	}
}
