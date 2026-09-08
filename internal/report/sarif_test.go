package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestSARIFContainsRuleAndResult(t *testing.T) {
	var out bytes.Buffer
	c := model.Comparison{
		Verdict: "BREAKING",
		Changes: []model.Change{
			{Surface: "filesystem", Kind: "target-conflict", Severity: "breaking", Message: "write denied"},
		},
	}
	if err := SARIF(&out, c); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `"version": "2.1.0"`) || !strings.Contains(text, "wabi.filesystem.target-conflict") {
		t.Fatalf("unexpected SARIF: %s", text)
	}
}
