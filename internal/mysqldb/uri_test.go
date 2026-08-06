package mysqldb

import "testing"

func TestDataSourceNameReadsAnAuthenticatorURI(t *testing.T) {
	t.Parallel()

	name, err := dataSourceName("mysql://authenticator:secret@127.0.0.1:3307/")
	if err != nil {
		t.Fatalf("dataSourceName: %v", err)
	}
	if want := "authenticator:secret@tcp(127.0.0.1:3307)/?parseTime=true"; name != want {
		t.Fatalf("name = %q, want %q", name, want)
	}
}

func TestDataSourceNameAddsTheMySQLPort(t *testing.T) {
	t.Parallel()

	name, err := dataSourceName("mysql://authenticator:secret@db.example/")
	if err != nil {
		t.Fatalf("dataSourceName: %v", err)
	}
	if want := "authenticator:secret@tcp(db.example:3306)/?parseTime=true"; name != want {
		t.Fatalf("name = %q, want %q", name, want)
	}
}

func TestDataSourceNameKeepsURIQueryValues(t *testing.T) {
	t.Parallel()

	name, err := dataSourceName("mysql://authenticator@127.0.0.1:3306/?tls=skip-verify")
	if err != nil {
		t.Fatalf("dataSourceName: %v", err)
	}
	if want := "authenticator:@tcp(127.0.0.1:3306)/?parseTime=true&tls=skip-verify"; name != want {
		t.Fatalf("name = %q, want %q", name, want)
	}
}

func TestDataSourceNameRefusesAURIThatIsNotMySQL(t *testing.T) {
	t.Parallel()

	for name, uri := range map[string]string{
		"another scheme":   "postgres://authenticator@127.0.0.1:5432/",
		"no user":          "mysql://127.0.0.1:3306/",
		"no host":          "mysql://authenticator@/",
		"not a URI at all": "mysql://authenticator@127.0.0.1:3306/%zz",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := dataSourceName(uri); err == nil {
				t.Fatalf("dataSourceName accepted %q", uri)
			}
		})
	}
}
