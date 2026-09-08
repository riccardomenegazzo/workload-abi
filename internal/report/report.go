package report

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
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
	fmt.Fprintf(w, "%s -> %s\n", c.Baseline, c.Candidate)
	if c.BaselineFingerprint != "" {
		fmt.Fprintf(w, "baseline fingerprint:  %s\n", c.BaselineFingerprint)
	}
	if c.CandidateFingerprint != "" {
		fmt.Fprintf(w, "candidate fingerprint: %s\n", c.CandidateFingerprint)
	}
	if c.Scenario != "" {
		fmt.Fprintf(w, "scenario: %s\n", c.Scenario)
	}
	if c.Target != "" {
		fmt.Fprintf(w, "target:   %s\n", c.Target)
	}
	if c.Policy != "" {
		fmt.Fprintf(w, "policy:   %s\n", c.Policy)
	}
	fmt.Fprintln(w)

	if len(c.Changes) == 0 {
		fmt.Fprintln(w, "No operational differences observed under this run.")
	} else {
		current := ""
		for _, ch := range c.Changes {
			if ch.Surface != current {
				if current != "" {
					fmt.Fprintln(w)
				}
				current = ch.Surface
				fmt.Fprintln(w, strings.ToUpper(current))
			}
			marker := "+"
			switch ch.Kind {
			case "removed":
				marker = "-"
			case "changed", "regression", "expanded", "target-conflict", "policy-violation":
				marker = "~"
			}
			fmt.Fprintf(w, "  %s [%s] %s\n", marker, strings.ToUpper(ch.Severity), ch.Message)
			if ch.Before != "" {
				fmt.Fprintf(w, "      before: %s\n", ch.Before)
			}
			if ch.After != "" {
				fmt.Fprintf(w, "      after:  %s\n", ch.After)
			}
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("-", 64))
	fmt.Fprintf(w, "RUNTIME COMPATIBILITY: %s\n", c.Verdict)
	return nil
}

func SARIF(w io.Writer, c model.Comparison) error {
	rules := map[string]map[string]any{}
	results := make([]map[string]any, 0, len(c.Changes))
	for _, ch := range c.Changes {
		ruleID := sarifRuleID(ch)
		if _, ok := rules[ruleID]; !ok {
			rules[ruleID] = map[string]any{
				"id": ruleID,
				"shortDescription": map[string]string{
					"text": ch.Surface + " " + ch.Kind,
				},
				"help": map[string]string{
					"text": "Workload ABI observed an operational compatibility change.",
				},
			}
		}
		result := map[string]any{
			"ruleId":  ruleID,
			"level":   sarifLevel(ch.Severity),
			"message": map[string]string{"text": ch.Message},
			"properties": map[string]any{
				"surface":  ch.Surface,
				"kind":     ch.Kind,
				"severity": ch.Severity,
				"before":   ch.Before,
				"after":    ch.After,
				"verdict":  c.Verdict,
				"scenario": c.Scenario,
				"target":   c.Target,
				"policy":   c.Policy,
			},
		}
		results = append(results, result)
	}

	ruleList := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		ruleList = append(ruleList, rule)
	}

	doc := map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []any{
			map[string]any{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":           "Workload ABI",
						"informationUri": "https://github.com/riccardomenegazzo/workload-abi",
						"rules":          ruleList,
					},
				},
				"results": results,
				"properties": map[string]any{
					"baseline":             c.Baseline,
					"candidate":            c.Candidate,
					"baselineFingerprint":  c.BaselineFingerprint,
					"candidateFingerprint": c.CandidateFingerprint,
					"scenario":             c.Scenario,
					"target":               c.Target,
					"policy":               c.Policy,
					"verdict":              c.Verdict,
				},
			},
		},
	}
	return JSON(w, doc)
}

var nonRule = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sarifRuleID(ch model.Change) string {
	return nonRule.ReplaceAllString("wabi."+ch.Surface+"."+ch.Kind, "-")
}

func sarifLevel(severity string) string {
	switch severity {
	case "breaking":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}
