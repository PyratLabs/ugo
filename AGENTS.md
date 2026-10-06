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
- `internal/output/output.go` — colored/emoji output with `--no-color` flag support; all printed messages and help text pass through `Sanitize`/`SanitizeInline` (control/bidi characters and invalid UTF-8 → U+FFFD)
- `internal/output/output_test.go` — ANSI stripping and sanitization verification
- `internal/args/args.go` — argument validation (enum, glob, regex) plus the shell-safety check (`ShellSafe`)
- `internal/trust/trust.go` — direnv-style trust store; content-hashed gate over the local config

Tests: `cmd/root_test.go`, `internal/{config,checker,version,output,args,trust}/*_test.go`

## Working conventions

- Verbs are defined in YAML, not hardcoded. Adding a new verb means editing config, not code.
- Config schema:
  - `shell_options: "set -euo pipefail"` — prepended to all shell scripts (every `cmd` and all `cmds` items)
  - `commands.<verb>.{cmd, cmds, env, description, group, arguments[], prompts[], platforms[]}` — `cmd` is a string, `cmds` is a list of strings, `env` is a map of environment variables, `group` names a help-output section; arguments are objects with `name`, optional `values` (enum), optional `match` (glob or regex), optional `exclude` (list of disallowed values), optional `raw: true` (opt out of the shell-safety check); prompts are objects with `name`, `description`, optional `sensitive`, optional `from_env_var`
  - `groups[]` — optional list of `{name, description}` defining help-output sections and their order; verbs opt in via `group: <name>`. Undeclared-but-referenced groups are auto-created; once any verb is grouped, ungrouped verbs go under "Other Commands" and built-ins under "Built-in Commands" (see `applyGroups` in `cmd/root.go`)
  - `tools.<binary>.{min_version, max_version, version_cmd, download_url}` — pre-flight checks run before every verb
- Arguments after the verb are mapped positionally to `arguments` entries and expanded into `${name}` placeholders in `cmd` or `cmds`
- `cmd` runs via `sh -c` (single-line and multiline block scalars alike), so quoting, pipes, and shell operators work; multiline blocks run as a shell script
- `cmds` is a list of commands; each item runs via `sh -c`, so shell features (variables, subshells, pipes) work; multi-line items also run as shell scripts
- `env` sets environment variables for the command execution; merged with current environment
- `match` is auto-detected: contains `*` or `?` → glob (checks files on disk); otherwise → regex (full string match, anchored as `^(?:pattern)$` so top-level alternation stays bound)
- Glob matching accepts full path, basename, basename without extension, or directory name
- `exclude` filters out values from glob matches and rejects them during validation; excluded values are hidden from help output
- Values accepted via `match` must also be shell-safe (`ShellSafe` in `internal/args`: `[A-Za-z0-9._@%+=:,/-]`, no spaces, non-empty) because they expand unquoted into `sh -c`; `raw: true` opts out, and help hides glob matches that fail the check. `values` enums (author-written) and unconstrained arguments are not checked
- Sensitive prompt names must match `^[A-Za-z_][A-Za-z0-9_]*$` (the shell expands them from the env); non-sensitive prompts expand on the Go side, so any name works
- Config commands that are reserved, have a key with whitespace or terminal-unsafe characters (`validCommandName`), or have an invalid sensitive prompt name are skipped with a warning (`skipReason` in `cmd/root.go`) — cobra dispatches on the first word of `Use`, so `"version 2.0"` would shadow the built-in `version`
- `ugo check` runs tool checks and prints status for each tool
- Version comparison uses `golang.org/x/mod/semver`; `version_cmd` output is scanned for a semver pattern
- Running a verb without required arguments (or with invalid args) prints the error then the help, then exits
- Security model: the local (CWD) config is gated by the trust store; the global config and argument/prompt values are trusted. `cmd`/`cmds` run via `sh -c` and `${name}` values are expanded as unquoted shell text — `match`-constrained args are additionally shell-checked (see above), unconstrained args are trusted verbatim. Sensitive prompt values are masked in uGo's output only — they still reach the shell (visible in `ps`, and to `set -x`). See README "Security".
- Trust gate: the local (CWD) config must be trusted before any verb or `check` executes. Trust is content-hashed (path + SHA-256) in `~/.config/<binary>/trust.json`; editing the config revokes it. `PersistentPreRunE` prompts on a TTY and shows a preview of everything the file executes (`shell_options`, each verb's script and `env`, tool `version_cmd`); a read error on the answer (e.g. EOF without a newline) denies. `--trust` records trust without prompting (CI/CD); non-interactive + untrusted aborts. `help`/`version` are never gated. The global config is implicitly trusted. A world-writable trust store, global config, or their directory triggers a warning (skipped on Windows).
