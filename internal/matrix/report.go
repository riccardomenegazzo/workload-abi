package matrix

import (
	"fmt"
	"io"
	"strings"
)

func WriteHuman(w io.Writer, artifact Artifact) error {
	if _, err := fmt.Fprintln(w, "WORKLOAD ABI — ENVIRONMENT COMPATIBILITY MATRIX"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("=", 72)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s -> %s\n", artifact.Baseline, artifact.Candidate); err != nil {
		return err
	}
	if artifact.Scenario != "" {
		if _, err := fmt.Fprintf(w, "scenario: %s\n", artifact.Scenario); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "fingerprint: %s\n\n", artifact.Fingerprint); err != nil {
		return err
	}

	for _, result := range artifact.Environments {
		surfaces := SortedBlockingSurfaces(result)
		detail := ""
		if len(surfaces) > 0 {
			detail = "  blockers=" + strings.Join(surfaces, ",")
		}
		target := result.Target
		if target == "" {
			target = "policy-only"
		}
		if _, err := fmt.Fprintf(w, "%-24s %-10s %-32s%s\n", result.Name, result.Verdict, target, detail); err != nil {
			return err
		}
		for _, change := range result.Changes {
			if !strings.EqualFold(change.Severity, "breaking") {
				continue
			}
			if _, err := fmt.Fprintf(w, "  - %s/%s: %s\n", change.Surface, change.Kind, change.Message); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(w, strings.Repeat("-", 72)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "MATRIX VERDICT: %s  compatible=%d changed=%d breaking=%d\n",
		artifact.Verdict, artifact.Summary.Compatible, artifact.Summary.Changed, artifact.Summary.Breaking)
	return err
}
