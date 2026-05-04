# AGENTS.md

## What is this repo?

- uGo — a Go CLI using Cobra that executes project-specific commands defined in YAML config.
- Binary is renameable; config file name and help text follow the binary name automatically.
- Config loading: global (`~/.config/<binary>/config.yaml`) merged with local (`./<binary>.yaml`), local overrides.

## Commands

```bash
go build -o ugo .          # build
go test ./...              # run all tests
go test ./... -cover       # run tests with coverage
go vet ./...               # vet
```

## Structure

- `main.go` — entry point, calls `cmd.RootCmd().Execute()`
- `cmd/root.go` — Cobra root command; dynamically creates subcommands from YAML config
- `internal/config/config.go` — loads global + local config, merges them
- `internal/checker/checker.go` — pre-flight tool dependency validation
- `internal/version/version.go` — semver extraction and comparison
- `internal/output/output.go` — colored/emoji output with `--no-color` flag support
- `internal/output/output_test.go` — ANSI stripping verification
- `internal/args/args.go` — argument validation (enum, glob, regex)

Tests: `cmd/root_test.go`, `internal/{config,checker,version,output,args}/*_test.go`

## Working conventions

- Verbs are defined in YAML, not hardcoded. Adding a new verb means editing config, not code.
- Config schema:
  - `commands.<verb>.{cmd, description, arguments[]}` — arguments are objects with `name`, optional `values` (enum), optional `match` (glob or regex), optional `exclude` (list of disallowed values)
  - `tools.<binary>.{min_version, max_version, version_cmd, download_url}` — pre-flight checks run before every verb
- Arguments after the verb are mapped positionally to `arguments` entries and expanded into `${name}` placeholders in `cmd`
- `cmd` supports multiline YAML block scalars (`|`); each non-empty line is executed sequentially
- `match` is auto-detected: contains `*` or `?` → glob (checks files on disk); otherwise → regex (full string match, auto-anchored)
- Glob matching accepts full path, basename, basename without extension, or directory name
- `exclude` filters out values from glob matches and rejects them during validation; excluded values are hidden from help output
- `ugo check` runs tool checks and prints status for each tool
- Version comparison uses `golang.org/x/mod/semver`; `version_cmd` output is scanned for a semver pattern
- Running a verb without required arguments (or with invalid args) prints the error then the help, then exits
