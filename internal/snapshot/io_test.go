package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestLoadVerifiesFingerprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	s := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "app:v1", Processes: []model.Process{{Command: "app"}}}
	s.Fingerprint = model.Fingerprint(s)
	data, _ := json.Marshal(s)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fingerprint != s.Fingerprint {
		t.Fatalf("fingerprint=%q want %q", got.Fingerprint, s.Fingerprint)
	}
}

func TestLoadRejectsTamperedSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	s := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "app:v1"}
	s.Fingerprint = model.Fingerprint(s)
	s.Processes = []model.Process{{Command: "tampered"}}
	data, _ := json.Marshal(s)
	_ = os.WriteFile(path, data, 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected fingerprint mismatch")
	}
}
