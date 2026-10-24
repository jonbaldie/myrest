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

func TestLoadRefusesUnquotedFileText(t *testing.T) {
	t.Parallel()

	for name, text := range map[string]string{
		"db-uri":                      "mysql://authenticator@127.0.0.1:3306/",
		"db-schemas":                  "myrest_fixture",
		"db-anon-role":                "myrest_anon",
		"jwt-secret":                  "secret",
		"jwt-aud":                     "audience",
		"jwt-role-claim-key":          ".role",
		"db-pre-request":              "app.check_request",
		"db-tx-end":                   "commit",
		"server-cors-allowed-origins": "https://example.com",
		"openapi-mode":                "disabled",
		"openapi-server-proxy-uri":    "https://example.com",
		"db-root-spec":                "app.root",
	} {
		for _, value := range []string{"'" + text + "'", text + " # main db", text, ""} {
			t.Run(name+"/"+value, func(t *testing.T) {
				t.Parallel()

				_, err := config.Load("# config\n"+name+" = "+value+"\n", config.Environment{})
				if err == nil {
					t.Fatal("Load accepted text without double quotes")
				}
				for _, want := range []string{"line 2", name, "quoted"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
			})
		}
	}
}

func TestLoadKeepsQuotedTextCharacters(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		value string
		want  string
	}{
		{value: `"'myrest_fixture' # literal"`, want: "'myrest_fixture' # literal"},
		{value: `""`, want: ""},
		{value: `"a\"b\\c"`, want: "a\"b\\c"},
	} {
		t.Run(testCase.value, func(t *testing.T) {
			t.Parallel()

			settings, err := config.Load("jwt-secret = "+testCase.value, config.Environment{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if settings.JWT.Secret != testCase.want {
				t.Fatalf("JWT.Secret = %q, want %q", settings.JWT.Secret, testCase.want)
			}
		})
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
