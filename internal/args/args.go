package args

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/PyratLabs/ugo/internal/config"
)

// Validate checks a single argument value against its validation rules.
// Returns nil if the argument is valid or has no rules.
func Validate(arg config.Argument, value string) error {
	if len(arg.Exclude) > 0 {
		if err := validateExclude(arg.Name, arg.Exclude, value); err != nil {
			return err
		}
	}

	if len(arg.Values) > 0 {
		if err := validateEnum(arg.Name, arg.Values, value); err != nil {
			return err
		}
	}

	if arg.Match != "" {
		if err := validateMatch(arg.Name, arg.Match, value); err != nil {
			return err
		}
	}

	return nil
}

func validateExclude(name string, exclude []string, actual string) error {
	for _, v := range exclude {
		if v == actual {
			return fmt.Errorf("argument '%s': value %q is not allowed", name, actual)
		}
	}
	return nil
}

func validateEnum(name string, values []string, actual string) error {
	for _, v := range values {
		if v == actual {
			return nil
		}
	}
	return fmt.Errorf("argument '%s': value %q not in allowed values: %s", name, actual, values)
}

func validateMatch(name string, pattern string, actual string) error {
	if IsGlob(pattern) {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("argument '%s': invalid glob pattern %q: %w", name, pattern, err)
		}
		for _, m := range matches {
			if m == actual || filepath.Base(m) == actual || stripExt(filepath.Base(m)) == actual {
				return nil
			}
			if dirMatch := filepath.Base(filepath.Dir(m)); dirMatch == actual {
				return nil
			}
		}
		return fmt.Errorf("argument '%s': no file matching pattern %q found for value %q", name, pattern, actual)
	}

	pattern = "^" + pattern + "$"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("argument '%s': invalid regex pattern %q: %w", name, pattern, err)
	}
	if !re.MatchString(actual) {
		return fmt.Errorf("argument '%s': value %q does not match pattern %q", name, actual, pattern)
	}

	return nil
}

func IsGlob(pattern string) bool {
	for _, c := range pattern {
		if c == '*' || c == '?' {
			return true
		}
	}
	return false
}

func stripExt(name string) string {
	ext := filepath.Ext(name)
	if ext != "" {
		return name[:len(name)-len(ext)]
	}
	return name
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

// ArgNames extracts argument names for usage display and error messages
func ArgNames(args []config.Argument) []string {
	names := make([]string, len(args))
	for i, a := range args {
		names[i] = a.Name
	}
	return names
}

// ValidateArgs validates all arguments in order
func ValidateArgs(arguments []config.Argument, values []string) []error {
	var errs []error
	for i, arg := range arguments {
		if i >= len(values) {
			break
		}
		if err := Validate(arg, values[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// ArgMap builds a name-to-value map for template expansion
func ArgMap(arguments []config.Argument, values []string) map[string]string {
	m := make(map[string]string)
	for i, arg := range arguments {
		if i < len(values) {
			m[arg.Name] = values[i]
		}
	}
	return m
}

// GlobMatches returns the list of display names from a glob pattern,
// filtered to exclude any values in the exclude list.
func GlobMatches(pattern string, exclude []string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}

	isDir := isDirGlob(pattern)
	excludeSet := make(map[string]bool, len(exclude))
	for _, v := range exclude {
		excludeSet[v] = true
	}

	seen := make(map[string]bool)
	var names []string
	for _, m := range matches {
		var name string
		if isDir {
			name = filepath.Base(filepath.Dir(m))
		} else {
			name = stripExt(filepath.Base(m))
		}
		if excludeSet[name] || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
