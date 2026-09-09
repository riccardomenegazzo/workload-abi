package target

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKubernetesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deployment.json")
	data := `{
	  "apiVersion":"apps/v1",
	  "kind":"Deployment",
	  "metadata":{"name":"api","namespace":"payments"},
	  "spec":{"template":{
	    "metadata":{"labels":{"app":"api","tier":"backend"}},
	    "spec":{
	      "terminationGracePeriodSeconds":12,
	      "securityContext":{"runAsNonRoot":true},
	      "containers":[{
	        "name":"api",
	        "securityContext":{"readOnlyRootFilesystem":true,"capabilities":{"drop":["ALL"]}},
	        "resources":{"limits":{"memory":"128Mi"}},
	        "volumeMounts":[{"name":"tmp","mountPath":"/tmp"}]
	      }]
	    }
	  }}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKubernetes(context.Background(), path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Deployment" || got.Name != "api" || got.Namespace != "payments" || got.Container != "api" {
		t.Fatalf("unexpected target identity: %#v", got)
	}
	if got.PodLabels["app"] != "api" || got.PodLabels["tier"] != "backend" {
		t.Fatalf("unexpected pod labels: %#v", got.PodLabels)
	}
	if !got.ReadOnlyRootfs || !got.RunAsNonRoot || got.MemoryLimit != 128*1024*1024 {
		t.Fatalf("unexpected target constraints: %#v", got)
	}
	if got.StopGracePeriod.Seconds() != 12 || len(got.WritablePaths) != 1 || got.WritablePaths[0] != "/tmp" {
		t.Fatalf("unexpected target runtime constraints: %#v", got)
	}
}

func TestLoadKubernetesDefaultsNamespaceAndReadsPodLabels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pod.json")
	data := `{
	  "apiVersion":"v1",
	  "kind":"Pod",
	  "metadata":{"name":"api","labels":{"app":"api"}},
	  "spec":{"containers":[{"name":"api"}]}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKubernetes(context.Background(), path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Namespace != "default" || got.PodLabels["app"] != "api" {
		t.Fatalf("unexpected pod identity: %#v", got)
	}
}

func TestDetectKubernetesAndCompose(t *testing.T) {
	dir := t.TempDir()
	kube := filepath.Join(dir, "kube.yaml")
	compose := filepath.Join(dir, "compose.yaml")
	_ = os.WriteFile(kube, []byte("apiVersion: v1\nkind: Pod\n"), 0o600)
	_ = os.WriteFile(compose, []byte("services:\n  app:\n    image: demo\n"), 0o600)
	if got, _ := Detect(kube); got != "kubernetes" {
		t.Fatalf("detect kube=%q", got)
	}
	if got, _ := Detect(compose); got != "compose" {
		t.Fatalf("detect compose=%q", got)
	}
}
