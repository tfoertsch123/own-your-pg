# Repository Guidelines

## Project Structure & Module Organization

`own-your-pg` is a Go module for capturing PostgreSQL logical replication
changes into crash-safe rotating files.

- `capture/` — core replication receiver: reads WAL via `pglogreplsimple`,
  writes JSON records to files, handles rotation and crash recovery
- `slot/` — slot management: on-disk slot files with magic, header, config,
  and PID locking
- `msg/` — generated message types (BEGIN, COMMIT, INSERT, UPDATE, DELETE,
  MESSAGE) with `go:generate`
- `lsn/` — PostgreSQL LSN type with parsing, formatting, and JSON/database
  scanning
- `defaults/` — shared constants (directory names, file names, size limits)
- `exe/` — entry points for binaries (e.g. `exe/capture`, `exe/slottool`)
- `jval/` — JSON value utilities

Dependencies on local modules are via `replace` directives in `go.mod`
(`../flock`, `../pglogreplsimple`).

## Build, Test, and Development Commands

```bash
make test          # run tests for all packages with coverage
make exe           # build all binaries into bin/
make Tcapture      # test a single package (prefix T + dir name)
make Gslot         # run go generate for slot/
make Gmsg          # run go generate for msg/
go vet ./...       # static analysis
```

Integration tests require a running PostgreSQL with `test_decoding` and
`wal2json` plugins. Set:

```bash
export OYPG_TEST_CONNINFO=postgres://user:pass@host:port/dbname
```

Tests without this env var are skipped automatically.

## Coding Style & Naming Conventions

- Tab width 4, tabs for indentation.
- Each go file should have this comment block at the end of the file:
  ```
  // Local Variables:
  // tab-width: 4
  // End:

  ```
- line length should not exceed 79 characters
- Error variables: `ErrFoo` package-level, returned not logged in library code
- Struct fields: lowercase, unexported where possible; accessed via methods
- Tests in the same package (`package capture`, not `package capture_test`)
- Generated files: `*_gen.go`, `*_xgen.go` — do not edit; run `go generate`

## Testing Guidelines

- Framework: standard `testing` package with `t.TempDir()` for filesystem
  isolation
- Unit tests: no external dependencies; use temp dirs and in-process slot
  files
- Integration tests: gated by `OYPG_TEST_CONNINFO` env var;
  use `test_decoding` plugin
- Test naming: `TestFunctionName_CaseName`
  (e.g. `TestReadSettings_SynchronousOn`)
- Subtests with `t.Run()` for table-driven cases
- Coverage profiles written to `<dir>/cover.out` by `make test`

## Commit & Pull Request Guidelines

Commit messages are lowercase, concise, and describe what changed:

```
capture: refactor reload/settings, fix metadata error handling, store flock
```

Multi-line messages wrap at ~72 columns. No conventional-commits prefix
(`feat:`, `fix:`) is used. Keep messages descriptive of the change, not
the file.
