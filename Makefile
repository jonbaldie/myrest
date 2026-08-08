.PHONY: test scenarios messgo mutago mysql-fixtures build verification-docs

# Run the Go test suite (HTTP seam + MySQL fixture harness).
# -p 1 keeps packages from sharing one local mysqld at the same time when
# Docker is unavailable and the harness falls back to port 3306.
test:
	go test -p 1 ./...

# Run every normative scenario at the HTTP seam (MySQL 8 acceptance package).
# See docs/verification.md for the scenario index, gap list, and smoke set.
scenarios:
	go test -p 1 ./test/acceptance ./internal/verification

# Rebuild docs/verification.md from capability-area Gap list rows and the
# scenario index. go test ./internal/verification fails when the doc drifts.
verification-docs:
	go run ./cmd/genverification

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
	mutago --coverage --min-covered-msi 80 --quiet --no-diffs \
		./internal/config ./internal/httpapi ./internal/jwt ./internal/mysqldb \
		./internal/readquery ./internal/rows ./internal/schemacache \
		./internal/verification

# Start MySQL 8.0+ in Docker and load fixture SQL. Ctrl+C stops the container.
mysql-fixtures:
	go run ./cmd/mysqlharness ./testdata/fixtures/schema.sql
