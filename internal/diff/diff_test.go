package diff

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestCompareFindsOperationalChanges(t *testing.T) {
	a := model.RuntimeGraph{Image:model.ImageIdentity{Reference:"app:v1"}, Processes:[]model.Process{{Name:"app"}}, Filesystem:[]model.FileMutation{{Kind:"A",Path:"/tmp/cache"}}, Resources:model.ResourceProfile{PeakMemoryBytes:32<<20}, Lifecycle:model.LifecycleProfile{ShutdownMillis:500}}
	b := model.RuntimeGraph{Image:model.ImageIdentity{Reference:"app:v2"}, Processes:[]model.Process{{Name:"app"},{Name:"sh"}}, Filesystem:[]model.FileMutation{{Kind:"A",Path:"/tmp/cache"},{Kind:"A",Path:"/var/lib/app/cache"}}, Resources:model.ResourceProfile{PeakMemoryBytes:80<<20}, Lifecycle:model.LifecycleProfile{ShutdownMillis:7000}}
	r := Compare(a,b)
	if r.Summary.Total < 4 { t.Fatalf("expected at least 4 changes, got %+v",r.Summary) }
	found := false
	for _,c := range r.Changes { if c.Surface=="filesystem" && c.Kind=="added" { found=true } }
	if !found { t.Fatal("expected filesystem change") }
}
