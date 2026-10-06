package cmd

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unicode"

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

// sensitiveMask replaces sensitive prompt references in displayed commands.
const sensitiveMask = "********"

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
	localCfg   *config.Config
	localRaw   []byte // raw bytes of the local config as parsed; nil when absent
	noColor    bool
	trustFlag  bool
)

func RootCmd() *cobra.Command {
	binaryName = config.BinaryName()

	var err error
	appCfg, localCfg, localRaw, err = config.Load(binaryName)
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

	// NO_COLOR (https://no-color.org) sets the default; --no-color=false
	// still overrides it explicitly.
	root.PersistentFlags().BoolVar(&noColor, "no-color", os.Getenv("NO_COLOR") != "", "disable color output")
	root.PersistentFlags().BoolVar(&trustFlag, "trust", false, "trust this directory's config without prompting (for CI/CD)")
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf("%s\nRun '%s help' for usage", err.Error(), binaryName)
	})

	// Build subcommands from config, skipping any that can't be registered
	// safely (see skipReason).
	verbs := make(map[string]*cobra.Command, len(appCfg.Commands))
	for name, cmdDef := range appCfg.Commands {
		if reason := skipReason(name, cmdDef); reason != "" {
			fmt.Fprintf(os.Stderr, "warning: ignoring config command %q: %s\n", name, reason)
			continue
		}
		verbs[name] = buildCommand(name, cmdDef)
		root.AddCommand(verbs[name])
	}

	check := checkCmd()
	ver := versionCmd()
	root.AddCommand(check)
	root.AddCommand(ver)

	applyGroups(root, verbs, appCfg, check, ver)

	return root
}

// Reserved group IDs for commands the config doesn't assign a group. Prefixed
// so they can't collide with a user-declared group name.
const (
	otherGroupID   = "__other__"
	builtinGroupID = "__builtin__"
)

// applyGroups wires config-defined command groups into cobra's help output.
// When no verb references a group, help output is unchanged. Otherwise every
// command is assigned a group: ungrouped verbs go under "Other Commands" and
// built-ins under "Built-in Commands", so cobra's default "Additional
// Commands" bucket (which would mix the two) never appears.
func applyGroups(root *cobra.Command, verbs map[string]*cobra.Command, cfg *config.Config, builtins ...*cobra.Command) {
	used := make(map[string]bool)
	for name, def := range cfg.Commands {
		if def.Group != "" && verbs[name] != nil {
			used[def.Group] = true
		}
	}
	if len(used) == 0 {
		return
	}

	// Declared groups keep their YAML order and use description as the help
	// section title. Groups without commands are skipped so help never shows
	// an empty heading.
	addGroup := func(id, title string) {
		if !root.ContainsGroup(id) {
			root.AddGroup(&cobra.Group{ID: id, Title: output.SanitizeInline(title) + ":"})
		}
	}
	for _, g := range cfg.Groups {
		if !used[g.Name] {
			continue
		}
		title := g.Description
		if title == "" {
			title = g.Name
		}
		addGroup(g.Name, title)
	}

	// Groups referenced by a verb but not declared are auto-created, appended
	// alphabetically after the declared ones. cobra panics at Execute() on a
	// GroupID with no registered group, so every used name must be added.
	extras := make([]string, 0, len(used))
	for name := range used {
		if !root.ContainsGroup(name) {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		addGroup(name, name)
	}

	hasUngrouped := false
	for name, c := range verbs {
		if group := cfg.Commands[name].Group; group != "" {
			c.GroupID = group
		} else {
			c.GroupID = otherGroupID
			hasUngrouped = true
		}
	}
	if hasUngrouped {
		addGroup(otherGroupID, "Other Commands")
	}

	addGroup(builtinGroupID, "Built-in Commands")
	for _, c := range builtins {
		c.GroupID = builtinGroupID
	}
	root.SetHelpCommandGroupID(builtinGroupID)
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

			fmt.Fprintf(os.Stdout, "%s\n\n", output.Bold("Checking tool dependencies..."))

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

// runToolChecks is the pre-flight gate before a verb runs. It only verifies
// tools exist on PATH — version constraints execute config-defined commands
// and can be slow, so they are enforced by the check command instead.
func runToolChecks() error {
	if len(appCfg.Tools) == 0 {
		return nil
	}

	issues := checker.CheckInstalled(appCfg.Tools)
	if len(issues) == 0 {
		return nil
	}

	output.CheckFail("Tool dependency errors:")
	for _, issue := range issues {
		for _, e := range issue.Errors {
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
	globalPath, localPath := config.ConfigPaths(binaryName)
	storePath, err := trust.DefaultStorePath(binaryName)
	if err != nil {
		return err
	}
	warnIfWritableByOthers(storePath, "trust decisions may be subverted")
	warnIfWritableByOthers(globalPath, "the global config is always trusted")
	warnIfWritableByOthers(filepath.Dir(storePath), "the trust store and global config can be replaced")
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	return trustGate(localPath, storePath, localRaw, localCfg, appCfg, bufio.NewReader(os.Stdin), os.Stderr, trustFlag, interactive)
}

// warnIfWritableByOthers prints a warning when path exists and is writable
// by other users. The trust store and global config are per-user security
// state; anyone who can write them can pre-approve arbitrary content.
func warnIfWritableByOthers(path, consequence string) {
	// Windows reports 0666 for every writable file; the bits mean nothing.
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// ponytail: world-write only. Group-write is the umask-002 default on
	// user-private-group systems, where flagging it is noise; check group
	// membership if shared-group setups matter.
	if info.Mode().Perm()&0o002 != 0 {
		fmt.Fprintf(os.Stderr, "warning: %s is writable by other users; %s\n", path, consequence)
	}
}

// trustPreview summarizes everything in the local config that executes, so
// the trust prompt shows what will actually run, not just an opaque path:
// shell_options, each verb's script and env, and tool version commands.
// Scripts come from the merged (platform-resolved) config, keyed by the local
// config's commands, so the preview matches execution.
func trustPreview(local, effective *config.Config) string {
	if local == nil {
		return ""
	}
	var b strings.Builder
	if local.ShellOptions != "" {
		fmt.Fprintf(&b, "    shell_options (prepended to every command, global ones too): %s\n", summarizeScript(local.ShellOptions))
	}

	header := false
	for _, name := range slices.Sorted(maps.Keys(local.Commands)) {
		def, ok := effective.Commands[name]
		// Skipped verbs never run, and their names may be unsafe to print.
		if skipReason(name, def) != "" {
			continue
		}
		if !header {
			b.WriteString("    Commands this file defines:\n")
			header = true
		}
		if !ok {
			fmt.Fprintf(&b, "      %s: (no command on this platform)\n", name)
			continue
		}
		scripts := def.Cmds
		if len(scripts) == 0 {
			scripts = []string{def.Cmd}
		}
		fmt.Fprintf(&b, "      %s: %s\n", name, summarizeScript(scripts...))
		if len(def.Env) > 0 {
			pairs := make([]string, 0, len(def.Env))
			for _, k := range slices.Sorted(maps.Keys(def.Env)) {
				pairs = append(pairs, k+"="+def.Env[k])
			}
			fmt.Fprintf(&b, "        env: %s\n", summarizeScript(strings.Join(pairs, " ")))
		}
	}

	header = false
	for _, name := range slices.Sorted(maps.Keys(local.Tools)) {
		if cmd := local.Tools[name].VersionCmd; cmd != "" {
			if !header {
				b.WriteString("    Tool version commands (run by check):\n")
				header = true
			}
			fmt.Fprintf(&b, "      %s: %s\n", output.SanitizeInline(name), summarizeScript(cmd))
		}
	}
	return b.String()
}

// summarizeScript renders the first non-blank line of scripts, annotated with
// how many more lines run, so the preview stays one line per entry without
// hiding that more executes. Whitespace runs are collapsed so padding cannot
// push a payload past the truncation point.
func summarizeScript(scripts ...string) string {
	var lines []string
	for _, s := range scripts {
		for _, l := range strings.Split(s, "\n") {
			if l = strings.Join(strings.Fields(output.SanitizeInline(l)), " "); l != "" {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 {
		return "(no-op)"
	}
	first := lines[0]
	const maxRunes = 100
	if r := []rune(first); len(r) > maxRunes {
		first = fmt.Sprintf("%s… (+%d chars)", string(r[:maxRunes]), len(r)-maxRunes)
	}
	if len(lines) > 1 {
		first += fmt.Sprintf(" (+%d more lines)", len(lines)-1)
	}
	return first
}

// trustGate decides whether the local config may be executed. raw is the
// content that was actually parsed (nil when no local config exists). The
// prompt previews what local defines, resolved against effective (see
// trustPreview), so the user sees what they are trusting. It returns nil to
// allow execution or an error explaining why it is blocked. allow
// corresponds to --trust; interactive reports whether prompting is possible.
func trustGate(localPath, storePath string, raw []byte, local, effective *config.Config, in *bufio.Reader, out io.Writer, allow, interactive bool) error {
	// Only the working-directory config is gated; the global config is
	// user-owned and implicitly trusted.
	if raw == nil {
		return nil
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	// Hash the bytes that were parsed, not the file as it is now: the file
	// can change between load and this check (e.g. while the prompt is
	// open), and recording a hash of unseen content would pre-trust it.
	hash := trust.HashBytes(raw)

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

	// The directory name comes from wherever the repo was unpacked.
	shownPath := output.SanitizeInline(absPath)
	fmt.Fprintln(out)
	if status == trust.Changed {
		fmt.Fprintf(out, "    ⚠️  %s has changed since it was last trusted.\n", shownPath)
	} else {
		fmt.Fprintf(out, "    ⚠️  %s is not trusted.\n", shownPath)
	}
	fmt.Fprintln(out, "    Running a verb here will execute the commands defined in this file.")
	if preview := trustPreview(local, effective); preview != "" {
		fmt.Fprintln(out)
		fmt.Fprint(out, preview)
	}
	fmt.Fprint(out, "    Trust it? [y/N]: ")

	line, err := in.ReadString('\n')
	// Any read error (EOF/Ctrl-D, interrupted read) denies: a partial line
	// like "y" without a newline must not grant trust.
	if err != nil {
		return fmt.Errorf("%s not trusted; aborting", localPath)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		if err := store.Trust(absPath, hash); err != nil {
			return fmt.Errorf("recording trust: %w", err)
		}
		fmt.Fprintf(out, "    trusted %s\n\n", shownPath)
		return nil
	default:
		return fmt.Errorf("%s not trusted; aborting", localPath)
	}
}

// shellVarNameRE matches valid shell variable names.
var shellVarNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// skipReason explains why a config command must not be registered, or
// returns "" when it may be. The trust preview uses it too, so it lists only
// verbs that can actually run.
func skipReason(name string, def config.Command) string {
	// Names that collide with built-ins would create ambiguous dispatch and,
	// for the trust-exempt built-ins, a path to run untrusted code.
	if reservedNames[name] {
		return "name is reserved"
	}
	if !validCommandName(name) {
		return "name must not contain whitespace or control characters"
	}
	for _, p := range def.Prompts {
		// Sensitive values reach the shell through the environment and are
		// expanded there, so the name must be a shell variable: "${api-key}"
		// would silently expand $api with a default of "key".
		if p.Sensitive && !shellVarNameRE.MatchString(p.Name) {
			return fmt.Sprintf("sensitive prompt %q: name must be a valid shell variable name", p.Name)
		}
	}
	return ""
}

// validCommandName reports whether a config command key is safe to register.
// cobra derives a command's dispatch name from the first word of Use, so a
// key like "version 2.0" would register as "version" and shadow the built-in;
// terminal-unsafe characters would be echoed raw into help output.
func validCommandName(name string) bool {
	return name != "" && output.SanitizeInline(name) == name && !strings.ContainsFunc(name, unicode.IsSpace)
}

func buildCommand(name string, def config.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   buildUse(name, def.Arguments),
		Short: output.SanitizeInline(def.Description),
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
			b.WriteString(fmt.Sprintf("  %-20s", output.SanitizeInline(arg.Name)))
			switch {
			case len(arg.Values) > 0:
				b.WriteString(output.SanitizeInline(strings.Join(arg.Values, ", ")))
			case arg.Match != "":
				if args.IsGlob(arg.Match) {
					matches := args.GlobMatches(arg.Match, arg.Exclude)
					// Hide names Validate would reject, so help lists exactly
					// what is accepted.
					if !arg.Raw {
						matches = slices.DeleteFunc(matches, func(m string) bool { return !args.ShellSafe(m) })
					}
					if len(matches) > 0 {
						b.WriteString(output.SanitizeInline(strings.Join(matches, ", ")))
					} else {
						b.WriteString("(no files found)")
					}
				} else {
					b.WriteString(output.SanitizeInline(fmt.Sprintf("^(?:%s)$", arg.Match)))
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
			b.WriteString(fmt.Sprintf("  %-20s", output.SanitizeInline(p.Name)))
			b.WriteString(output.SanitizeInline(p.Description))
			if p.FromEnvVar != "" {
				b.WriteString(fmt.Sprintf(" (or $%s)", output.SanitizeInline(p.FromEnvVar)))
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
		parts[i] = fmt.Sprintf("<%s>", output.SanitizeInline(arg.Name))
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
	secretEnv := make(map[string]string)

	// Collect interactive prompt values
	for _, p := range def.Prompts {
		var value string

		// Check from_env_var first
		if p.FromEnvVar != "" {
			if envVal, ok := os.LookupEnv(p.FromEnvVar); ok {
				value = envVal
			}
		}

		// Prompt if there is still no value. Note a set-but-empty from_env_var
		// (e.g. TOKEN="") intentionally falls through to the interactive
		// prompt — this matches the documented "unset or empty" behavior, so
		// an empty env var cannot be used to supply an empty answer.
		if value == "" {
			var err error
			if p.Sensitive {
				value, err = readSensitiveInput(p.Description)
			} else {
				value, err = readInput(p.Description)
			}
			if err != nil {
				output.CheckFail(fmt.Sprintf("failed to read input for '%s': %v", p.Name, err))
				os.Exit(1)
			}
		}

		// Sensitive values never enter the command text: the script becomes
		// the argv of "sh -c", which is world-readable in /proc while it
		// runs. They ride in the child environment instead, and the ${name}
		// reference is left literal for the shell to expand.
		if p.Sensitive {
			secretEnv[p.Name] = value
		} else {
			vars[p.Name] = value
		}
	}

	// env: values may reference sensitive prompts — the environment is only
	// readable by the same user, unlike argv, so real values are safe here.
	allVars := make(map[string]string, len(vars)+len(secretEnv))
	maps.Copy(allVars, vars)
	maps.Copy(allVars, secretEnv)
	expandedEnv := expandEnv(def.Env, allVars)

	childEnv := secretEnv
	maps.Copy(childEnv, expandedEnv) // an explicit env: key wins over an injected prompt

	// The displayed command masks sensitive references; the executed script
	// keeps them as literal ${name} for the shell to expand from childEnv.
	displayVars := maps.Clone(vars)
	for name := range secretEnv {
		displayVars[name] = sensitiveMask
	}

	shellOpts := appCfg.ShellOptions

	if len(def.Cmds) > 0 {
		return executeCmdsList(name, def.Cmds, vars, displayVars, childEnv, shellOpts)
	}

	return executeCmdString(name, def.Cmd, vars, displayVars, childEnv, shellOpts)
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

func executeCmdsList(name string, cmdsList []string, vars, displayVars, env map[string]string, shellOpts string) error {
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

func executeCmdString(name, cmdStr string, vars, displayVars, env map[string]string, shellOpts string) error {
	expanded := expandVars(cmdStr, vars)

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
		output.CommandRunning(name, expandVars(cmdStr, displayVars))
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
