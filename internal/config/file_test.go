package config_test

import (
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestLoadRefusesAKnobThatIsNotOnTheNormativeSurface(t *testing.T) {
	t.Parallel()

	file := `db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-channel = "pgrst"
`
	_, err := config.Load(file, config.Environment{})
	if err == nil {
		t.Fatal("Load accepted the dropped knob db-channel")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "db-channel") {
		t.Fatalf("error %q does not name line 2 and db-channel", err)
	}
}

func TestLoadRefusesALineThatIsNotAKnobValuePair(t *testing.T) {
	t.Parallel()

	_, err := config.Load("db-uri\n", config.Environment{})
	if err == nil {
		t.Fatal("Load accepted a line without a value")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error %q does not name line 1", err)
	}
}

func TestLoadRefusesAValueWithAnUnclosedQuote(t *testing.T) {
	t.Parallel()

	_, err := config.Load(`db-anon-role = "myrest_anon`, config.Environment{})
	if err == nil {
		t.Fatal("Load accepted a value with an unclosed quote")
	}
	if !strings.Contains(err.Error(), "db-anon-role") {
		t.Fatalf("error %q does not name db-anon-role", err)
	}
}

func TestLoadRefusesASingleQuotedTextValue(t *testing.T) {
	t.Parallel()

	_, err := config.Load(`db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = 'myrest_fixture'
db-anon-role = "myrest_anon"
`, config.Environment{})
	if err == nil {
		t.Fatal("Load accepted a single-quoted text value")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "db-schemas") {
		t.Fatalf("error %q does not name line 2 and db-schemas", err)
	}
}

func TestLoadRefusesABareTextValue(t *testing.T) {
	t.Parallel()

	_, err := config.Load(`db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-schemas = myrest_fixture # main db
db-anon-role = "myrest_anon"
`, config.Environment{})
	if err == nil {
		t.Fatal("Load accepted a bare text value")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "double-quoted") {
		t.Fatalf("error %q does not name line 2 and the quoting rule", err)
	}
}

func TestLoadAcceptsBareBooleanAndIntegerValues(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(`db-uri = "mysql://authenticator@127.0.0.1:3306/"
db-anon-role = "myrest_anon"
db-aggregates-enabled = true
jwt-cache-max-entries = 5
`, config.Environment{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !settings.DB.AggregatesEnabled || settings.JWT.CacheMaxEntries != 5 {
		t.Fatalf("bare values not applied: %+v", settings)
	}
}

func TestLoadIgnoresCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	file := `
# myrest.conf

	# an indented comment
db-anon-role = "myrest_anon"
`
	settings, err := config.Load(file, config.Environment{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.DB.AnonRole != "myrest_anon" {
		t.Fatalf("DB.AnonRole = %q, want myrest_anon", settings.DB.AnonRole)
	}
}
