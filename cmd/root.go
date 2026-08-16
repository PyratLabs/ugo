package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PyratLabs/ugo/internal/args"
	"github.com/PyratLabs/ugo/internal/checker"
	"github.com/PyratLabs/ugo/internal/config"
	"github.com/PyratLabs/ugo/internal/output"
	"github.com/PyratLabs/ugo/internal/trust"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var version = "dev"

// Annotation keys used to mark which commands may execute config-defined
// shell. Trust and tool checks are gated on these annotations rather than on
// the command name: names come from the (untrusted) config, so gating on the
// name would let a config command called "help" or "version" impersonate a
// built-in and skip the trust prompt entirely.
const (
	annExecutesConfig = "ugo/executes-config"
	annRunsToolChecks = "ugo/runs-tool-checks"
)

// reservedNames are built-in command names that a config must not redefine.
// Allowing a config to shadow them creates ambiguous dispatch and, for the
// trust-exempt built-ins, a path to run untrusted code.
var reservedNames = map[string]bool{
	"help":       true,
	"version":    true,
	"check":      true,
	"completion": true,
}

var (
	binaryName string
	appCfg     *config.Config
	noColor    bool
	trustFlag  bool
)

func RootCmd() *cobra.Command {
	binaryName = config.BinaryName()

	var err error
	appCfg, err = config.Load(binaryName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   binaryName,
		Short: fmt.Sprintf("%s — context-aware project verbs", binaryName),
		Long: fmt.Sprintf(`%s executes project-specific commands defined in YAML configuration.

Global config:  %s
Local config:   %s

Local config overrides global config for the same verb names.`,
			binaryName,
			func() string { g, _ := config.ConfigPaths(binaryName); return g }(),
			func() string { _, l := config.ConfigPaths(binaryName); return l }(),
		),
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			output.SetNoColor(noColor)
			// Only commands that can execute config-defined shell are gated,
			// identified by annotation rather than by name. Built-ins such as
			// help and version carry no annotation and run freely; a config
			// command cannot opt out of the gate by choosing their name.
			if cmd.Annotations[annExecutesConfig] != "true" {
				return nil
			}
			// These commands run config-defined commands, so the local config
			// must be trusted first.
			if err := enforceTrust(); err != nil {
				output.CheckFail(err.Error())
				os.Exit(1)
			}
			// check does its own tool checking in its Run.
			if cmd.Annotations[annRunsToolChecks] != "true" {
				return nil
			}
			return runToolChecks()
		},
	}

	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	root.PersistentFlags().BoolVar(&trustFlag, "trust", false, "trust this directory's config without prompting (for CI/CD)")
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf("unknown flag: %s\nRun '%s help' for usage", err.Error(), binaryName)
	})

	// Build subcommands from config. Names that collide with built-in
	// commands are skipped: they would create ambiguous dispatch and, for the
	// trust-exempt built-ins, could otherwise be used to run untrusted code.
	for name, cmdDef := range appCfg.Commands {
		if reservedNames[name] {
			fmt.Fprintf(os.Stderr, "warning: ignoring config command %q: name is reserved\n", name)
			continue
		}
		root.AddCommand(buildCommand(name, cmdDef))
	}

	root.AddCommand(checkCmd())
	root.AddCommand(versionCmd())

	return root
}

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check required tool dependencies",
		// check runs config-defined version commands, so it must be trusted.
		// It performs its own tool checking, so it omits annRunsToolChecks.
		Annotations: map[string]string{annExecutesConfig: "true"},
		Run: func(cmd *cobra.Command, args []string) {
			output.SetNoColor(noColor)
			if len(appCfg.Tools) == 0 {
				output.Info("No tool dependencies configured.")
				return
			}

			output.Bold("Checking tool dependencies...\n")

			issues := checker.CheckTools(appCfg.Tools)
			printToolStatus(appCfg.Tools, issues)

			if checker.HasErrors(issues) {
				fmt.Fprintln(os.Stderr)
				output.CheckFail("Tool checks failed")
				os.Exit(1)
			}

			fmt.Fprintln(os.Stderr)
			output.CheckPass("All tool dependencies satisfied")
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}

func printToolStatus(tools map[string]config.Tool, issues []checker.Issue) {
	issueMap := make(map[string]checker.Issue)
	for _, i := range issues {
		issueMap[i.Tool] = i
	}

	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if issue, ok := issueMap[name]; ok {
			for _, e := range issue.Errors {
				if strings.HasPrefix(e, "version:") {
					output.CheckPass(fmt.Sprintf("%s (%s)", name, strings.TrimPrefix(e, "version: ")))
				} else {
					output.CheckFail(e)
				}
			}
		} else {
			output.CheckPass(name)
		}
	}
}

func runToolChecks() error {
	if len(appCfg.Tools) == 0 {
		return nil
	}

	issues := checker.CheckTools(appCfg.Tools)
	if !checker.HasErrors(issues) {
		return nil
	}

	output.CheckFail("Tool dependency errors:")
	for _, issue := range issues {
		for _, e := range issue.Errors {
			if strings.HasPrefix(e, "version:") {
				continue
			}
			output.CheckFail(fmt.Sprintf("%s: %s", issue.Tool, e))
		}
	}
	os.Exit(1)
	return nil
}

// enforceTrust gates execution of config-defined commands behind the trust
// store, prompting on os.Stdin or honoring --trust. It is thin glue over
// trustGate so the latter stays free of globals and easy to test.
func enforceTrust() error {
	_, localPath := config.ConfigPaths(binaryName)
	storePath, err := trust.DefaultStorePath(binaryName)
	if err != nil {
		return err
	}
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	return trustGate(localPath, storePath, bufio.NewReader(os.Stdin), os.Stderr, trustFlag, interactive)
}

// trustGate decides whether the local config may be executed. It returns nil to
// allow execution or an error explaining why it is blocked. allow corresponds
// to --trust; interactive reports whether prompting is possible.
func trustGate(localPath, storePath string, in *bufio.Reader, out io.Writer, allow, interactive bool) error {
	// Only the working-directory config is gated; the global config is
	// user-owned and implicitly trusted.
	if !fileExists(localPath) {
		return nil
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	hash, err := trust.HashFile(localPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", localPath, err)
	}

	store, err := trust.Load(storePath)
	if err != nil {
		return fmt.Errorf("loading trust store: %w", err)
	}

	status := store.Status(absPath, hash)
	if status == trust.Trusted {
		return nil
	}

	// --trust: skip the prompt and record trust (best-effort, so a read-only
	// home in CI/CD doesn't fail the run).
	if allow {
		if err := store.Trust(absPath, hash); err != nil {
			fmt.Fprintf(out, "    warning: could not record trust for %s: %v\n", absPath, err)
		}
		return nil
	}

	if !interactive {
		return fmt.Errorf("%s is not trusted; re-run with --trust to allow it (e.g. in CI/CD)", localPath)
	}

	fmt.Fprintln(out)
	if status == trust.Changed {
		fmt.Fprintf(out, "    ⚠️  %s has changed since it was last trusted.\n", absPath)
	} else {
		fmt.Fprintf(out, "    ⚠️  %s is not trusted.\n", absPath)
	}
	fmt.Fprintln(out, "    Running a verb here will execute the commands defined in this file.")
	fmt.Fprint(out, "    Trust it? [y/N]: ")

	line, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		if err := store.Trust(absPath, hash); err != nil {
			return fmt.Errorf("recording trust: %w", err)
		}
		fmt.Fprintf(out, "    trusted %s\n\n", absPath)
		return nil
	default:
		return fmt.Errorf("%s not trusted; aborting", localPath)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func buildCommand(name string, def config.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   buildUse(name, def.Arguments),
		Short: def.Description,
		Long:  buildLong(def.Arguments, def.Prompts),
		// Config verbs execute config-defined shell and run pre-flight tool
		// checks, so both trust and tool checks are enforced before RunE.
		Annotations: map[string]string{
			annExecutesConfig: "true",
			annRunsToolChecks: "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeCommand(cmd, name, def, args)
		},
	}

	return c
}

func buildLong(arguments []config.Argument, prompts []config.Prompt) string {
	if len(arguments) == 0 && len(prompts) == 0 {
		return ""
	}

	var b strings.Builder

	if len(arguments) > 0 {
		b.WriteString("\nArguments:\n")
		for _, arg := range arguments {
			b.WriteString(fmt.Sprintf("  %-20s", arg.Name))
			switch {
			case len(arg.Values) > 0:
				b.WriteString(strings.Join(arg.Values, ", "))
			case arg.Match != "":
				if args.IsGlob(arg.Match) {
					matches := args.GlobMatches(arg.Match, arg.Exclude)
					if len(matches) > 0 {
						b.WriteString(strings.Join(matches, ", "))
					} else {
						b.WriteString("(no files found)")
					}
				} else {
					b.WriteString(fmt.Sprintf("^(?:%s)$", arg.Match))
				}
			default:
				b.WriteString("(no validation)")
			}
			b.WriteString("\n")
		}
	}

	if len(prompts) > 0 {
		b.WriteString("\nPrompts:\n")
		for _, p := range prompts {
			b.WriteString(fmt.Sprintf("  %-20s", p.Name))
			b.WriteString(p.Description)
			if p.FromEnvVar != "" {
				b.WriteString(fmt.Sprintf(" (or $%s)", p.FromEnvVar))
			}
			if p.Sensitive {
				b.WriteString(" (sensitive)")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func buildUse(name string, arguments []config.Argument) string {
	if len(arguments) == 0 {
		return name
	}
	parts := make([]string, len(arguments))
	for i, arg := range arguments {
		parts[i] = fmt.Sprintf("<%s>", arg.Name)
	}
	return fmt.Sprintf("%s %s", name, strings.Join(parts, " "))
}

func executeCommand(cmd *cobra.Command, name string, def config.Command, values []string) error {
	if len(values) != len(def.Arguments) {
		argNames := args.ArgNames(def.Arguments)
		output.CheckFail(fmt.Sprintf("expected %d argument(s) for '%s': %s",
			len(def.Arguments), name, strings.Join(argNames, ", ")))
		fmt.Fprintln(os.Stderr)
		cmd.Help()
		os.Exit(1)
	}

	if errs := args.ValidateArgs(def.Arguments, values); len(errs) > 0 {
		for _, e := range errs {
			output.CheckFail(e.Error())
		}
		fmt.Fprintln(os.Stderr)
		cmd.Help()
		os.Exit(1)
	}

	vars := args.ArgMap(def.Arguments, values)
	sensitiveNames := make(map[string]bool)

	// Collect interactive prompt values
	for _, p := range def.Prompts {
		var value string

		// Check from_env_var first
		if p.FromEnvVar != "" {
			if envVal, ok := os.LookupEnv(p.FromEnvVar); ok {
				value = envVal
				if p.Sensitive {
					sensitiveNames[p.Name] = true
				}
			}
		}

		// Prompt if there is still no value. Note a set-but-empty from_env_var
		// (e.g. TOKEN="") intentionally falls through to the interactive
		// prompt — this matches the documented "unset or empty" behavior, so
		// an empty env var cannot be used to supply an empty answer.
		if value == "" {
			var err error
			if p.Sensitive {
				sensitiveNames[p.Name] = true
				value, err = readSensitiveInput(p.Description)
			} else {
				value, err = readInput(p.Description)
			}
			if err != nil {
				output.CheckFail(fmt.Sprintf("failed to read input for '%s': %v", p.Name, err))
				os.Exit(1)
			}
		}

		vars[p.Name] = value
	}

	// Expand env values against argument and prompt vars
	expandedEnv := expandEnv(def.Env, vars)
	shellOpts := appCfg.ShellOptions

	if len(def.Cmds) > 0 {
		return executeCmdsList(name, def.Cmds, vars, expandedEnv, shellOpts, sensitiveNames)
	}

	return executeCmdString(name, def.Cmd, vars, expandedEnv, shellOpts, sensitiveNames)
}

func readInput(description string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", description)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(input, "\n\r"), nil
}

func readSensitiveInput(description string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", description)
	bytepw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(bytepw), nil
}

func executeCmdsList(name string, cmdsList []string, vars map[string]string, env map[string]string, shellOpts string, sensitiveNames map[string]bool) error {
	displayVars := output.MaskedVars(vars, sensitiveNames)

	for i, cmdStr := range cmdsList {
		expanded := expandVars(cmdStr, vars)
		display := expandVars(cmdStr, displayVars)

		if len(cmdsList) > 1 {
			output.CommandRunning(fmt.Sprintf("%s (%d/%d)", name, i+1, len(cmdsList)), display)
		} else {
			output.CommandRunning(name, display)
		}

		if err := runShellScript(expanded, env, shellOpts); err != nil {
			output.CommandFail(name)
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			os.Exit(1)
		}
	}

	output.CommandSuccess(name)
	return nil
}

func executeCmdString(name, cmdStr string, vars map[string]string, env map[string]string, shellOpts string, sensitiveNames map[string]bool) error {
	expanded := expandVars(cmdStr, vars)
	displayVars := output.MaskedVars(vars, sensitiveNames)
	display := expandVars(cmdStr, displayVars)

	if strings.TrimSpace(expanded) == "" {
		output.CommandSuccess(name)
		return nil
	}

	// Single-line and multiline cmd both run via "sh -c" so that quoting,
	// embedded whitespace, and shell operators (&&, |, redirects) behave as
	// written rather than being split on whitespace into argv.
	if strings.Contains(expanded, "\n") {
		output.CommandRunning(name, "shell script")
	} else {
		output.CommandRunning(name, display)
	}

	if err := runShellScript(expanded, env, shellOpts); err != nil {
		output.CommandFail(name)
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}

	output.CommandSuccess(name)
	return nil
}

func expandVars(s string, vars map[string]string) string {
	return os.Expand(s, func(key string) string {
		val, ok := vars[key]
		if !ok {
			return fmt.Sprintf("${%s}", key)
		}
		return val
	})
}

// expandEnv expands ${var} references in all env values against the vars map.
func expandEnv(env map[string]string, vars map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	expanded := make(map[string]string, len(env))
	for k, v := range env {
		expanded[k] = expandVars(v, vars)
	}
	return expanded
}

func runShellScript(script string, env map[string]string, shellOpts string) error {
	if shellOpts != "" {
		script = shellOpts + "\n" + script
	}
	command := exec.Command("sh", "-c", script)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	applyEnv(command, env)
	return command.Run()
}

func applyEnv(cmd *exec.Cmd, env map[string]string) {
	if len(env) == 0 {
		return
	}
	environ := os.Environ()
	for k, v := range env {
		environ = append(environ, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = environ
}
