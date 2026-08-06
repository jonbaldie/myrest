package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestServeGateRefusesAnEmptyMinimumRunSetAndNamesEveryMissingKnob(t *testing.T) {
	t.Parallel()

	err := config.Settings{}.ServeGate()
	if err == nil {
		t.Fatal("ServeGate accepted settings with an empty minimum run set")
	}

	message := err.Error()
	for _, knob := range []string{"db-uri", "db-schemas", "jwt-secret", "db-anon-role"} {
		if !strings.Contains(message, knob) {
			t.Errorf("gate message %q does not name the missing knob %q", message, knob)
		}
	}
}

func TestServeGateAcceptsJWTSecretAloneAsTheAuthPartOfTheRunSet(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			URI:     "mysql://authenticator@127.0.0.1:3306/",
			Schemas: []string{"myrest_fixture"},
		},
		JWT: config.JWTSettings{Secret: "reallyreallyreallyreallyverysafe"},
	}

	if err := settings.ServeGate(); err != nil {
		t.Fatalf("ServeGate refused a complete run set with jwt-secret only: %v", err)
	}
}

func TestServeGateAcceptsAnonymousDatabaseRoleAloneAsTheAuthPartOfTheRunSet(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			URI:      "mysql://authenticator@127.0.0.1:3306/",
			Schemas:  []string{"myrest_fixture"},
			AnonRole: "myrest_anon",
		},
	}

	if err := settings.ServeGate(); err != nil {
		t.Fatalf("ServeGate refused a complete run set with db-anon-role only: %v", err)
	}
}

func TestServeGateNeedsOnlyTheKnobThatIsMissing(t *testing.T) {
	t.Parallel()

	settings := config.Settings{
		DB: config.DatabaseSettings{
			Schemas:  []string{"myrest_fixture"},
			AnonRole: "myrest_anon",
		},
	}

	err := settings.ServeGate()
	if err == nil {
		t.Fatal("ServeGate accepted settings without db-uri")
	}

	var incomplete *config.IncompleteRunSetError
	if !errors.As(err, &incomplete) {
		t.Fatalf("ServeGate error %v is not an *IncompleteRunSetError", err)
	}
	if !reflect.DeepEqual(incomplete.Needs, []string{"db-uri"}) {
		t.Fatalf("Needs = %v, want [db-uri]", incomplete.Needs)
	}
}
