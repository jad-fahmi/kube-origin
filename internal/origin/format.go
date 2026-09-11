package origin

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func WriteText(w io.Writer, report Report) error {
	selection := "selected by current-context"
	if report.ContextSelection == "--context flag" {
		selection = "selected by --context"
	} else if report.ContextSelectionSource != "" {
		selection += " in " + printable(displayPath(report.ContextSelectionSource))
	}
	if _, err := fmt.Fprintf(w, "Context:        %s (%s)\n", printable(report.Context), selection); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Context entry:  %s\n", sourceOrUnknown(report.ContextSource)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Cluster:        %s (from %s)\n", valueOrUnset(report.Cluster), sourceOrUnknown(report.ClusterSource)); err != nil {
		return err
	}
	if report.Server != "" {
		if _, err := fmt.Fprintf(w, "API server:     %s (from %s)\n", printable(report.Server), sourceOrUnknown(report.ServerSource)); err != nil {
			return err
		}
	}
	if report.NamespaceDefault {
		if _, err := fmt.Fprintf(w, "Namespace:      %s (Kubernetes default)\n", printable(report.Namespace)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "Namespace:      %s (from %s)\n", printable(report.Namespace), sourceOrUnknown(report.NamespaceSource)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "User:           %s (from %s)\n", valueOrUnset(report.User), sourceOrUnknown(report.UserSource)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Authentication: %s\n", authSummary(report.Authentication)); err != nil {
		return err
	}

	if len(report.Inputs) > 0 {
		if _, err := fmt.Fprintln(w, "\nInputs, in precedence order:"); err != nil {
			return err
		}
		for i, input := range report.Inputs {
			status := ""
			if !input.Exists {
				status = " (missing; ignored by client-go)"
			}
			if _, err := fmt.Fprintf(w, "  %d. %s%s\n", i+1, printable(displayPath(input.Path)), status); err != nil {
				return err
			}
		}
	}
	if len(report.Shadowed) > 0 {
		if _, err := fmt.Fprintln(w, "\nShadowed entries:"); err != nil {
			return err
		}
		for _, entry := range report.Shadowed {
			if _, err := fmt.Fprintf(w, "  %s %q in %s was ignored; %s wins\n", entry.Kind, entry.Name, printable(displayPath(entry.IgnoredSource)), printable(displayPath(entry.WinningSource))); err != nil {
				return err
			}
		}
	}
	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "  %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

func sourceOrUnknown(source string) string {
	if source == "" {
		return "source unknown"
	}
	return printable(displayPath(source))
}

func valueOrUnset(value string) string {
	if value == "" {
		return "(not set)"
	}
	return printable(value)
}

func authSummary(auth Authentication) string {
	if auth.Exec != "" {
		return "exec plugin: " + printable(auth.Exec) + " (not executed)"
	}
	if len(auth.Methods) == 0 {
		return "unknown"
	}
	return strings.Join(auth.Methods, ", ")
}

func printable(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '\ufffd'
		}
		return r
	}, value)
}

func displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if relative, relErr := filepath.Rel(home, path); relErr == nil && relative == "." {
			return "~"
		} else if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "~" + string(filepath.Separator) + relative
		}
	}
	return path
}
