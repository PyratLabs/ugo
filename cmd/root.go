package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/PyratLabs/ugo/internal/args"
	"github.com/PyratLabs/ugo/internal/checker"
	"github.com/PyratLabs/ugo/internal/config"
	"github.com/PyratLabs/ugo/internal/output"
)

var (
	binaryName string
	appCfg     *config.Config
	noColor    bool
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
			if cmd.Name() == "check" || cmd.Name() == "help" {
				return nil
			}
			return runToolChecks()
		},
	}

	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf("unknown flag: %s\nRun '%s help' for usage", err.Error(), binaryName)
	})

	// Build subcommands from config
	for name, cmdDef := range appCfg.Commands {
		root.AddCommand(buildCommand(name, cmdDef))
	}

	root.AddCommand(checkCmd())

	return root
}

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check required tool dependencies",
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

func printToolStatus(tools map[string]config.Tool, issues []checker.Issue) {
	issueMap := make(map[string]checker.Issue)
	for _, i := range issues {
		issueMap[i.Tool] = i
	}

	for name := range tools {
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

func buildCommand(name string, def config.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   buildUse(name, def.Arguments),
		Short: def.Description,
		Long:  buildLong(def.Arguments),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeCommand(cmd, name, def, args)
		},
	}

	return c
}

func buildLong(arguments []config.Argument) string {
	if len(arguments) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nArguments:\n")
	for _, arg := range arguments {
		b.WriteString(fmt.Sprintf("  %-20s", arg.Name))
		switch {
		case len(arg.Values) > 0:
			b.WriteString(strings.Join(arg.Values, ", "))
		case arg.Match != "":
			if args.IsGlob(arg.Match) {
				matches, _ := filepath.Glob(arg.Match)
				if len(matches) > 0 {
					names := make([]string, len(matches))
					for i, m := range matches {
						if isDirGlob(arg.Match) {
							names[i] = filepath.Base(filepath.Dir(m))
						} else {
							base := filepath.Base(m)
							names[i] = strings.TrimSuffix(base, filepath.Ext(base))
						}
					}
					b.WriteString(strings.Join(names, ", "))
				} else {
					b.WriteString("(no files found)")
				}
			} else {
				b.WriteString(fmt.Sprintf("^%s$", arg.Match))
			}
		default:
			b.WriteString("(no validation)")
		}
		b.WriteString("\n")
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

	expanded := os.Expand(def.Cmd, func(key string) string {
		val, ok := vars[key]
		if !ok {
			return fmt.Sprintf("${%s}", key)
		}
		return val
	})

	parts := strings.Fields(expanded)
	if len(parts) == 0 {
		output.CheckFail(fmt.Sprintf("empty command configured for '%s'", name))
		os.Exit(1)
	}

	bin := parts[0]
	cmdArgs := parts[1:]

	output.CommandRunning(name, expanded)

	command := exec.Command(bin, cmdArgs...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	if err := command.Run(); err != nil {
		output.CommandFail(name)
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}

	output.CommandSuccess(name)
	return nil
}

func isDirGlob(pattern string) bool {
	dir := filepath.Dir(pattern)
	for _, c := range dir {
		if c == '*' || c == '?' {
			return true
		}
	}
	return false
}
