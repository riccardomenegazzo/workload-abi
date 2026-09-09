package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectECSTaskDefinition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.json")
	if err := os.WriteFile(path, []byte(`{"family":"payments","containerDefinitions":[{"name":"api"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ecs" {
		t.Fatalf("Detect()=%q want ecs", got)
	}
}
