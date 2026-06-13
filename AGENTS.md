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
  - `shell_options: "set -euo pipefail"` — prepended to all shell scripts (every `cmd` and all `cmds` items)
  - `commands.<verb>.{cmd, cmds, env, description, arguments[]}` — `cmd` is a string, `cmds` is a list of strings, `env` is a map of environment variables; arguments are objects with `name`, optional `values` (enum), optional `match` (glob or regex), optional `exclude` (list of disallowed values)
  - `tools.<binary>.{min_version, max_version, version_cmd, download_url}` — pre-flight checks run before every verb
- Arguments after the verb are mapped positionally to `arguments` entries and expanded into `${name}` placeholders in `cmd` or `cmds`
- `cmd` runs via `sh -c` (single-line and multiline block scalars alike), so quoting, pipes, and shell operators work; multiline blocks run as a shell script
- `cmds` is a list of commands; each item runs via `sh -c`, so shell features (variables, subshells, pipes) work; multi-line items also run as shell scripts
- `env` sets environment variables for the command execution; merged with current environment
- `match` is auto-detected: contains `*` or `?` → glob (checks files on disk); otherwise → regex (full string match, anchored as `^(?:pattern)$` so top-level alternation stays bound)
- Glob matching accepts full path, basename, basename without extension, or directory name
- `exclude` filters out values from glob matches and rejects them during validation; excluded values are hidden from help output
- `ugo check` runs tool checks and prints status for each tool
- Version comparison uses `golang.org/x/mod/semver`; `version_cmd` output is scanned for a semver pattern
- Running a verb without required arguments (or with invalid args) prints the error then the help, then exits
- Security model: config (global + local-from-CWD) and argument/prompt values are trusted; `cmd`/`cmds` run via `sh -c` and `${name}` values are expanded as unquoted shell text. Constrain untrusted args with `values`/`match`. Sensitive prompt values are masked in uGo's output only — they still reach the shell (visible in `ps`, and to `set -x`). See README "Security".
