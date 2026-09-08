package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func Human(w io.Writer, c model.Comparison) error {
	fmt.Fprintln(w, "WORKLOAD ABI")
	fmt.Fprintln(w, strings.Repeat("=", 64))
	fmt.Fprintf(w, "%s -> %s\n\n", c.Baseline, c.Candidate)
	if len(c.Changes) == 0 {
		fmt.Fprintln(w, "No operational differences observed under this run.")
	} else {
		current := ""
		for _, ch := range c.Changes {
			if ch.Surface != current {
				if current != "" { fmt.Fprintln(w) }
				current = ch.Surface
				fmt.Fprintln(w, strings.ToUpper(current))
			}
			marker := "+"
			switch ch.Kind { case "removed": marker = "-"; case "changed", "regression", "expanded": marker = "~" }
			fmt.Fprintf(w, "  %s [%s] %s\n", marker, strings.ToUpper(ch.Severity), ch.Message)
			if ch.Before != "" { fmt.Fprintf(w, "      before: %s\n", ch.Before) }
			if ch.After != "" { fmt.Fprintf(w, "      after:  %s\n", ch.After) }
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("-", 64))
	fmt.Fprintf(w, "RUNTIME COMPATIBILITY: %s\n", c.Verdict)
	return nil
}
