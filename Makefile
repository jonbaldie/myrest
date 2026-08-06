.PHONY: test messgo mutago mysql-fixtures build

# Run the Go test suite (HTTP seam + MySQL fixture harness).
test:
	go test ./...

# Build the myrest service binary.
build:
	go build -o bin/myrest ./cmd/myrest

# CODING_STANDARDS.md: messgo design + codesize with no violations.
messgo:
	messgo ./internal,./cmd text design,codesize --ignore-tests

# CODING_STANDARDS.md: mutago covered-MSI of 80% or higher on production packages.
# The MySQL Docker harness is test infrastructure and is not mutated here.
# cmd/myrest is glue over these packages; its tests run it as a process, so
# mutation coverage of the command comes from the packages it calls.
mutago:
	mutago --coverage --min-covered-msi 80 --quiet --no-diffs ./internal/config ./internal/httpapi

# Start MySQL 8.0+ in Docker and load fixture SQL. Ctrl+C stops the container.
mysql-fixtures:
	go run ./cmd/mysqlharness ./testdata/fixtures/schema.sql
