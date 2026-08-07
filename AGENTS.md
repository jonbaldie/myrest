Only reply in ASD-STE100 Simplified Technical English.

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `jonbaldie/myrest` (via `gh`). See `docs/agents/issue-tracker.md`.

### Triage labels

Default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout (`CONTEXT.md` + `docs/adr/` at repo root). See `docs/agents/domain.md`.

## Cursor Cloud specific instructions

`myrest` is one Go service. It gives an HTTP API over MySQL. The standard commands are in `README.md` and the `Makefile`.

### Docker daemon

The MySQL harness (`internal/mysqltest`) and the acceptance tests in `test/acceptance` start a `mysql:8.0` Docker container. Docker has no `systemd` here, so a new session must start the daemon by hand:

```bash
sudo dockerd >/tmp/dockerd.log 2>&1 &
sudo chmod 666 /var/run/docker.sock   # let the ubuntu user reach the socket
```

The daemon uses the `fuse-overlayfs` storage driver with the containerd snapshotter off (`/etc/docker/daemon.json`); Docker 29 needs the snapshotter off for `fuse-overlayfs`. Without a running daemon, `make test`, `make mysql-fixtures`, and `make mutago` stop with a Docker error.

### PATH for messgo and mutago

The update script puts `messgo` and `mutago` in `$(go env GOPATH)/bin` (`~/go/bin`). `~/.bashrc` adds that directory to `PATH`, so a login shell finds the tools for `make messgo` and `make mutago`. A non-login shell must add it first:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Run the service end to end

`make mysql-fixtures` starts MySQL and prints a random host port. Its `DSN`/`RootDSN` show the `myrest`/`root` logins, but `myrest` must connect as the **authenticator** account of the fixture SQL, not those logins. Point `db-uri` at that account:

```bash
# with PORT from the make mysql-fixtures output
printf 'db-uri = "mysql://authenticator:secret@127.0.0.1:%s/"\ndb-schemas = "myrest_fixture"\ndb-anon-role = "myrest_anon"\n' "$PORT" > /tmp/myrest.conf
MYREST_LISTEN=127.0.0.1:3000 ./bin/myrest /tmp/myrest.conf
curl http://127.0.0.1:3000/items    # -> [{"id":1,"name":"alpha",...}]
```
