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

func TestLoadAcceptsV1Alpha2FingerprintAfterSchemaUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	s := model.Snapshot{
		SchemaVersion: model.SchemaVersionV1Alpha2,
		Image:         "legacy:v1",
		Processes:     []model.Process{{Command: "legacy"}},
		Filesystem:    []model.FilesystemChange{{Kind: "A", Path: "/tmp/legacy"}},
	}
	s.Fingerprint = model.Fingerprint(s)
	data, _ := json.Marshal(s)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("legacy v1alpha2 snapshot should remain loadable: %v", err)
	}
	if got.SchemaVersion != model.SchemaVersionV1Alpha2 {
		t.Fatalf("legacy schema changed during load: %q", got.SchemaVersion)
	}
}

func TestLoadRejectsV1Alpha2ClaimingDeepEvents(t *testing.T) {
	s := model.Snapshot{
		SchemaVersion: model.SchemaVersionV1Alpha2,
		Image:         "legacy:v1",
		RuntimeEvents: []model.RuntimeEvent{{Category: "file", Operation: "open", Target: "/etc/passwd"}},
	}
	if err := Validate(s); err == nil {
		t.Fatal("expected v1alpha2 deep evidence rejection")
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
