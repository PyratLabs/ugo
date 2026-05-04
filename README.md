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
go install github.com/xanmanning/ugo@latest
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
ugo plan --help           # shows argument validation rules
ugo --no-color            # disable colored output
```

## Configuration

uGo loads configuration from two locations and merges them (local overrides global):

| Scope    | Path                                       |
|----------|--------------------------------------------|
| Global   | `~/.config/<binary>/config.yaml`           |
| Local    | `<project-root>/<binary>.yaml`             |

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

#### Commands

```yaml
commands:
  <verb>:
    cmd: "<shell command with ${arg} templates>"
    description: "<short help text>"
    arguments:                     # positional argument definitions (optional)
      - name: <arg1>
        values: [val1, val2]       # optional: restrict to enum values
        match: "<glob or regex>"   # optional: validate with file glob or regex
```

#### Argument validation

Arguments support three validation modes:

| Mode | Config | Behavior |
|------|--------|----------|
| **Enum** | `values: [dev, staging, prod]` | Value must be in the list |
| **Glob** | `match: "playbooks/*.yaml"` | Checks files on disk; accepts full path, basename, or basename without extension |
| **Regex** | `match: "[a-z]+"` | Full-string regex match (auto-anchored) |

Glob vs regex is auto-detected: if the pattern contains `*` or `?` it's treated as a file glob.

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

Running a verb without required arguments shows the error followed by argument validation rules:

```bash
$ ugo plan

❌ expected 2 argument(s) for 'plan': environment, playbook

Arguments:
  environment         dev, staging, prod
  playbook            ensure-ssh, setup-db

Usage:
  ugo plan <environment> <playbook> [flags]
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
