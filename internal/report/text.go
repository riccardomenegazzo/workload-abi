package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func CompareText(w io.Writer, r model.CompareResult) {
	fmt.Fprintln(w, "WORKLOAD ABI")
	fmt.Fprintln(w, strings.Repeat("─", 64))
	fmt.Fprintf(w, "%s → %s\n\n", r.Diff.From.Reference, r.Diff.To.Reference)
	if len(r.Diff.Changes) == 0 {
		fmt.Fprintln(w, "No operational changes observed under this scenario.")
	} else {
		current := ""
		for _, c := range r.Diff.Changes {
			surface := strings.ToUpper(c.Surface)
			if surface != current {
				if current != "" { fmt.Fprintln(w) }
				fmt.Fprintln(w, surface)
				current = surface
			}
			fmt.Fprintf(w, "  %-8s %-9s %s\n", symbol(c.Severity), strings.ToUpper(string(c.Severity)), c.Summary)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Changes: %d total · %d critical · %d high · %d medium · %d low\n", r.Diff.Summary.Total, r.Diff.Summary.Critical, r.Diff.Summary.High, r.Diff.Summary.Medium, r.Diff.Summary.Low)
	if r.Compatibility != nil {
		c := r.Compatibility
		fmt.Fprintln(w, "\nTARGET COMPATIBILITY")
		fmt.Fprintln(w, strings.Repeat("─", 64))
		fmt.Fprintf(w, "Environment: %s", c.Environment)
		if c.Service != "" { fmt.Fprintf(w, " (service: %s)", c.Service) }
		fmt.Fprintln(w)
		for _, x := range c.Conflicts {
			fmt.Fprintf(w, "  %s %-10s %s\n      constraint: %s\n      observed:   %s\n", symbol(x.Severity), strings.ToUpper(x.Surface), x.Explanation, x.Constraint, x.Observed)
		}
		for _, n := range c.Notes { fmt.Fprintf(w, "  · %s\n", n) }
		fmt.Fprintln(w)
		fmt.Fprintf(w, "RUNTIME COMPATIBILITY: %s\n", strings.ToUpper(string(c.Status)))
	}
}

func symbol(s model.ChangeSeverity) string {
	switch s {
	case model.SeverityCritical: return "✗"
	case model.SeverityHigh: return "!"
	case model.SeverityMedium: return "△"
	default: return "·"
	}
}
