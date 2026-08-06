package mysqltest

import (
	"fmt"
	"os"
	"path/filepath"
)

// FixtureSchema is the path of the fixture SQL, from a test package one or two
// directories below the repository root.
func FixtureSchema(root string) string {
	return filepath.Join(root, "testdata", "fixtures", "schema.sql")
}

// RunTests gives one started MySQL 8 database, with the fixture SQL loaded, to
// run, and stops the database when run gives back the exit code of the tests.
// A TestMain function passes that code to os.Exit.
func RunTests(fixtures []string, run func(*Harness) int) int {
	harness, err := Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start MySQL: %v\n", err)
		return 1
	}
	defer func() {
		if err := harness.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "stop MySQL: %v\n", err)
		}
	}()

	if err := harness.LoadSQL(fixtures...); err != nil {
		fmt.Fprintf(os.Stderr, "load the fixtures: %v\n", err)
		return 1
	}
	return run(harness)
}
