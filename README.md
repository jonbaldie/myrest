# myrest

myrest will be an HTTP API service that exposes MySQL 8.0+ with PostgREST-compatible client contracts.

This repository is in **prefactor**: the Go module, quality gates, MySQL fixture harness, and HTTP test seam are in place. The running service does **not** claim PostgREST parity yet (`GET /` returns `"parity":"none"`).

## Requirements

- Go 1.26+
- Docker (for the MySQL 8.0+ test harness)
- [`messgo`](https://github.com/quality-gates/messgo) and [`mutago`](https://github.com/quality-gates/mutago) on `PATH`

```bash
go install github.com/quality-gates/messgo/cmd/messgo@latest
go install github.com/quality-gates/mutago/v2/cmd/mutago@latest
```

## Commands

| Command | Purpose |
| --- | --- |
| `go run ./cmd/myrest` | Start the myrest service (`MYREST_LISTEN`, default `127.0.0.1:3000`) |
| `make test` | Run tests (HTTP seam + MySQL harness) |
| `make messgo` | Run messgo `design` and `codesize` rulesets (must report no violations) |
| `make mutago` | Run mutago on `./internal/httpapi` with `--coverage --min-covered-msi 80` |
| `make mysql-fixtures` | Start MySQL 8.0+ and load `testdata/fixtures/schema.sql` |

## Fixture DDL

`testdata/fixtures/schema.sql` creates database `myrest_fixture` and table `items` for later parity slices. Parent-spec fixtures stay intent-only; this file is the concrete DDL for the harness.
