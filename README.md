# uGo

The ubiquitous `./go` script — as a Go binary.

## Overview

uGo is a CLI tool that executes project-specific commands defined in YAML configuration.
It follows the ThoughtWorks `./go` script pattern: provide a common vocabulary for each
project, with verbs that are context-aware based on the current working directory.

The binary name is flexible — rename it and the help text and config file name follow
automatically, allowing multiple instances for different projects.

## Quick Start

### Install

```bash
go install github.com/PyratLabs/ugo@latest
```

Or build from source:

```bash
go build -o ugo .
```

### Create a config

Create `<binary-name>.yaml` in your project root:

```yaml
tools:
  ansible-playbook:
    min_version: "2.10.0"
    version_cmd: "ansible-playbook --version"
    download_url: "https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html"
  go:
    min_version: "1.25.0"
    version_cmd: "go version"
  kubectl:
    download_url: "https://kubernetes.io/docs/tasks/tools/"

commands:
  plan:
    cmd: ansible-playbook --check environments/${environment}/inventory.yaml playbooks/${playbook}.yaml
    description: "Run an ansible playbook in check mode."
    arguments:
      - name: environment
        match: "environments/*/inventory.yaml"
      - name: playbook
        match: "playbooks/*.yaml"
  lint:
    cmd: go test ./...
    description: "Run test suite"
  deploy:
    cmd: kubectl apply -f ./deployments/${service} --context ${region}
    description: "Deploy a service"
    arguments:
      - name: service
        match: "[a-z][a-z0-9-]+"
      - name: region
        values: [us-east-1, eu-west-1, ap-southeast-1]
```

### Run commands

```bash
ugo plan dev ensure-ssh   # runs: ansible-playbook --check environments/dev/inventory.yaml playbooks/ensure-ssh.yaml
ugo lint                  # runs: go test ./...
ugo check                 # verify tool dependencies
ugo version               # print the version
ugo plan --help           # shows argument validation rules
ugo --no-color            # disable colored output
```

## Configuration

uGo loads configuration from two locations and merges them (local overrides global):

| Scope    | Path                                       |
|----------|--------------------------------------------|
| Global   | `~/.config/<binary>/config.yaml`           |
| Local    | `<project-root>/<binary>.yaml`             |

### Shell options

Set `shell_options` to prepend shell flags to all commands that run via `sh -c` (multiline `cmd` and all `cmds` items):

```yaml
shell_options: "set -euo pipefail"
```

This enables strict mode: exit on error (`-e`), unset variable error (`-u`), and pipe failure detection (`-o pipefail`).

### Config format

#### Tools

```yaml
tools:
  <binary>:
    min_version: "<semver>"        # optional minimum version
    max_version: "<semver>"        # optional maximum version
    version_cmd: "<cmd>"           # how to get the version string (defaults to "<binary> --version")
    download_url: "<url>"          # shown if tool is missing (optional)
```

If no `min_version` or `max_version` is specified, uGo only checks that the tool exists in `$PATH`:

```yaml
tools:
  terraform:                       # just check it exists
    download_url: "https://terraform.io/downloads"
```

#### Commands

```yaml
commands:
  <verb>:
    cmd: "<single command with ${arg} templates>"      # string: runs directly (single-line) or as shell script (multi-line)
    cmds:                                              # list: each item runs via sh -c (supports shell features)
      - "echo ${arg}"
      - |
        echo "multi-line"
        echo "script"
    env:                                               # optional: environment variables (supports ${arg} / ${prompt} expansion)
      MY_VAR: "value"
      TOKEN: "${api_token}"
    description: "<short help text>"
    arguments:                     # positional argument definitions (optional)
      - name: <arg1>
        values: [val1, val2]       # optional: restrict to enum values
        match: "<glob or regex>"   # optional: validate with file glob or regex
    prompts:                       # interactive prompts (optional)
      - name: api_token
        description: "Enter your API token"
        sensitive: true            # optional: masks input and display
```

#### Prompts

Prompts collect user input interactively at runtime. Values are referenced in `cmd`/`cmds`/`env` via `${name}` templates, just like arguments.

```yaml
commands:
  deploy:
    cmd: kubectl apply -f ${manifest} --token ${api_token}
    description: "Deploy with an API token"
    arguments:
      - name: manifest
        match: "*.yaml"
    prompts:
      - name: api_token
        description: "Enter API token"
        sensitive: true
```

| Field | Description |
|-------|-------------|
| `name` | Template variable name (used as `${name}` in commands) |
| `description` | Question shown to the user at the prompt |
| `sensitive` | If `true`, input is hidden during entry and displayed as `********` in command output; the real value is passed to the command |
| `from_env_var` | If set, the prompt is skipped when this environment variable is already set; its value is used directly. Falls through to interactive prompt when unset or empty |

Running the above command will prompt for the token, mask it on screen, and expand both `${manifest}` and `${api_token}` in the command:

```bash
$ ugo deploy deployment.yaml
Enter API token:         # input is hidden
🚀 deploy: kubectl apply -f deployment.yaml --token ********
```

With `from_env_var`, if the environment variable is set, no prompt appears:

```bash
$ API_TOKEN=sk-abc123 ugo deploy deployment.yaml
🚀 deploy: kubectl apply -f deployment.yaml --token ********
```

The help output shows the env var fallback:

```bash
Prompts:
  api_token           Enter API token (or $API_TOKEN) (sensitive)
```

#### Environment variable expansion

`env` values support `${name}` template expansion from both arguments and prompts:

```yaml
commands:
  deploy:
    cmds:
      - 'echo "$MY_TOKEN"'
    env:
      MY_TOKEN: "${api_token}"
    prompts:
      - name: api_token
        description: "Enter API token"
        sensitive: true
```

#### Argument validation

Arguments support three validation modes:

| Mode | Config | Behavior |
|------|--------|----------|
| **Enum** | `values: [dev, staging, prod]` | Value must be in the list |
| **Glob** | `match: "playbooks/*.yaml"` | Checks files on disk; accepts full path, basename, basename without extension, or directory name |
| **Regex** | `match: "[a-z]+"` | Full-string regex match (auto-anchored) |
| **Exclude** | `exclude: [default]` | Disallowed values (hidden from help output) |

Glob vs regex is auto-detected: if the pattern contains `*` or `?` it's treated as a file glob.

### Multiline commands

**`cmd` with multiline** — runs the entire block as a shell script via `sh -c`:

```yaml
commands:
  plan:
    cmd: |
      terraform workspace select -or-create=true "${workspace}"
      terraform plan
    description: "Run a terraform plan"
```

**`cmds` list** — each item runs via `sh -c`, so shell features (variables, subshells, pipes) work:

```yaml
commands:
  deploy:
    cmds:
      - 'echo "Deploying to $ENV"'
      - |
        echo "Running in subshell"
        (
          cd /tmp && pwd
        )
    env:
      ENV: "production"
    description: "Deploy with shell features"
```

### Exclude values

Combine `match` with `exclude` to hide certain values from glob matches:

```yaml
commands:
  plan:
    cmd: terraform workspace select "${workspace}" && terraform plan
    arguments:
      - name: workspace
        match: config/*.yaml
        exclude: [default]
```

Excluded values are rejected during validation and hidden from help output.

### Example

```bash
# config: cmd: ansible-playbook environments/${environment}/inventory.yaml
# args:   environment with match "environments/*/inventory.yaml"

ugo plan dev    →   ansible-playbook environments/dev/inventory.yaml
ugo plan test   →   ❌ argument 'environment': no file matching pattern "environments/*/inventory.yaml" found for value "test"
```

### Enum values

```bash
# config: cmd: kubectl apply -f ./deployments/${service} --context ${region}
# args:   region with values [us-east-1, eu-west-1, ap-southeast-1]

ugo deploy api us-east-1   →   kubectl apply -f ./deployments/api --context us-east-1
ugo deploy api us-west-2   →   ❌ argument 'region': value "us-west-2" not in allowed values: [us-east-1, eu-west-1, ap-southeast-1]
```

Running a verb without required arguments shows the error followed by argument and prompt validation rules:

```bash
$ ugo plan

❌ expected 2 argument(s) for 'plan': environment, playbook

Arguments:
  environment         dev, staging, prod
  playbook            ensure-ssh, setup-db

Usage:
  ugo plan <environment> <playbook> [flags]
```

When prompts are defined, they appear in the help output:

```bash
$ ugo deploy --help

Arguments:
  manifest            *.yaml

Prompts:
  api_token           Enter API token (sensitive)

Usage:
  ugo deploy <manifest> [flags]
```

## Tool Dependency Checks

Before executing any verb, uGo checks configured tools:

- Verifies each binary exists in `$PATH`
- Extracts version from `version_cmd` output
- Compares against `min_version` and `max_version` using semver

Run `ugo check` manually to inspect all tool status.

```bash
$ ugo check
  ✅ ansible-playbook (v2.20.5)
  ✅ docker (v28.5.1)
  ✅ All tool dependencies satisfied

$ ugo check
  ❌ nonexistent-tool is not installed, download at: https://example.com
  ✅ docker (v28.5.1)

  ❌ Tool checks failed
```

## Colored Output

uGo uses UTF-8 icons and colors for status output. Use `--no-color` to disable:

```bash
ugo --no-color plan dev ensure-ssh
```

## Further Reading

- [In Praise of the `./go` Script — Part I](https://www.thoughtworks.com/insights/blog/praise-go-script-part-i)
- [In Praise of the `./go` Script — Part II](https://www.thoughtworks.com/insights/blog/praise-go-script-part-ii)

## Author

Xan Manning, 2020
