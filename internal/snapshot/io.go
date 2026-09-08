package snapshot

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func Load(path string) (model.Snapshot, error) {
	var s model.Snapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("read snapshot: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse snapshot: %w", err)
	}
	if err := Validate(s); err != nil {
		return s, err
	}
	computed := model.Fingerprint(s)
	if s.Fingerprint == "" {
		s.Fingerprint = computed
	} else if s.Fingerprint != computed {
		return s, fmt.Errorf("snapshot fingerprint mismatch: stored %s computed %s", s.Fingerprint, computed)
	}
	return s, nil
}

func Validate(s model.Snapshot) error {
	if s.Image == "" {
		return fmt.Errorf("snapshot image is required")
	}
	if s.SchemaVersion != "" && s.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("unsupported snapshot schema %q", s.SchemaVersion)
	}
	return nil
}
