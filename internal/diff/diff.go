package diff

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func Compare(a, b model.RuntimeGraph) model.DiffResult {
	r := model.DiffResult{SchemaVersion: model.SchemaVersion, From: a.Image, To: b.Image, Scenario: b.Scenario}
	compareProcesses(&r, a, b)
	compareNetwork(&r, a, b)
	compareFilesystem(&r, a, b)
	comparePrivileges(&r, a, b)
	compareResources(&r, a, b)
	compareLifecycle(&r, a, b)
	sort.SliceStable(r.Changes, func(i, j int) bool {
		if severityRank(r.Changes[i].Severity) == severityRank(r.Changes[j].Severity) {
			return r.Changes[i].Surface+r.Changes[i].Key < r.Changes[j].Surface+r.Changes[j].Key
		}
		return severityRank(r.Changes[i].Severity) > severityRank(r.Changes[j].Severity)
	})
	for _, c := range r.Changes {
		r.Summary.Total++
		switch c.Severity {
		case model.SeverityCritical:
			r.Summary.Critical++
		case model.SeverityHigh:
			r.Summary.High++
		case model.SeverityMedium:
			r.Summary.Medium++
		case model.SeverityLow:
			r.Summary.Low++
		default:
			r.Summary.Info++
		}
	}
	return r
}

func compareProcesses(r *model.DiffResult, a, b model.RuntimeGraph) {
	ma, mb := map[string]bool{}, map[string]bool{}
	for _, p := range a.Processes { ma[p.Name+"|"+p.Command] = true }
	for _, p := range b.Processes { mb[p.Name+"|"+p.Command] = true }
	for k := range mb {
		if !ma[k] { r.Changes = append(r.Changes, model.Change{Surface:"process", Kind:"added", Key:k, After:k, Severity:model.SeverityMedium, Summary:"new runtime process observed: "+humanProcess(k)}) }
	}
	for k := range ma {
		if !mb[k] { r.Changes = append(r.Changes, model.Change{Surface:"process", Kind:"removed", Key:k, Before:k, Severity:model.SeverityLow, Summary:"runtime process no longer observed: "+humanProcess(k)}) }
	}
}

func compareNetwork(r *model.DiffResult, a, b model.RuntimeGraph) {
	ma, mb := map[string]model.NetworkEndpoint{}, map[string]model.NetworkEndpoint{}
	for _, n := range a.Network { ma[networkKey(n)] = n }
	for _, n := range b.Network { mb[networkKey(n)] = n }
	for k, n := range mb {
		if _, ok := ma[k]; !ok {
			sev := model.SeverityMedium
			if n.Direction == "outbound" { sev = model.SeverityHigh }
			r.Changes = append(r.Changes, model.Change{Surface:"network", Kind:"added", Key:k, After:n, Severity:sev, Summary:"new "+n.Direction+" network behavior: "+networkHuman(n)})
		}
	}
	for k, n := range ma {
		if _, ok := mb[k]; !ok { r.Changes = append(r.Changes, model.Change{Surface:"network", Kind:"removed", Key:k, Before:n, Severity:model.SeverityLow, Summary:"network behavior no longer observed: "+networkHuman(n)}) }
	}
}

func compareFilesystem(r *model.DiffResult, a, b model.RuntimeGraph) {
	ma, mb := map[string]model.FileMutation{}, map[string]model.FileMutation{}
	for _, f := range a.Filesystem { ma[f.Kind+"|"+f.Path] = f }
	for _, f := range b.Filesystem { mb[f.Kind+"|"+f.Path] = f }
	for k, f := range mb {
		if _, ok := ma[k]; !ok {
			sev := model.SeverityMedium
			if strings.HasPrefix(f.Path,"/etc/") || strings.HasPrefix(f.Path,"/root/") || strings.Contains(f.Path,"secret") || strings.Contains(f.Path,"credential") { sev = model.SeverityHigh }
			r.Changes = append(r.Changes, model.Change{Surface:"filesystem", Kind:"added", Key:k, After:f, Severity:sev, Summary:fmt.Sprintf("new filesystem mutation %s %s", f.Kind, f.Path)})
		}
	}
	for k, f := range ma {
		if _, ok := mb[k]; !ok { r.Changes = append(r.Changes, model.Change{Surface:"filesystem", Kind:"removed", Key:k, Before:f, Severity:model.SeverityLow, Summary:fmt.Sprintf("filesystem mutation no longer observed: %s %s", f.Kind, f.Path)}) }
	}
}

func comparePrivileges(r *model.DiffResult, a, b model.RuntimeGraph) {
	oldCaps, newCaps := set(a.Privileges.EffectiveCaps), set(b.Privileges.EffectiveCaps)
	for c := range newCaps {
		if !oldCaps[c] {
			sev := model.SeverityHigh
			if c=="SYS_ADMIN" || c=="SYS_MODULE" || c=="BPF" || c=="SYS_PTRACE" { sev = model.SeverityCritical }
			r.Changes = append(r.Changes, model.Change{Surface:"privilege", Kind:"added", Key:"cap:"+c, After:c, Severity:sev, Summary:"new effective Linux capability: "+c})
		}
	}
	if a.Privileges.EffectiveUID != b.Privileges.EffectiveUID {
		sev := model.SeverityMedium
		if b.Privileges.EffectiveUID==0 && a.Privileges.EffectiveUID!=0 { sev = model.SeverityCritical }
		r.Changes = append(r.Changes, model.Change{Surface:"privilege", Kind:"changed", Key:"effective_uid", Before:a.Privileges.EffectiveUID, After:b.Privileges.EffectiveUID, Severity:sev, Summary:fmt.Sprintf("effective UID changed from %d to %d", a.Privileges.EffectiveUID, b.Privileges.EffectiveUID)})
	}
	if a.Image.User != b.Image.User { r.Changes = append(r.Changes, model.Change{Surface:"privilege", Kind:"changed", Key:"image_user", Before:a.Image.User, After:b.Image.User, Severity:model.SeverityHigh, Summary:fmt.Sprintf("image runtime user changed from %q to %q", a.Image.User, b.Image.User)}) }
}

func compareResources(r *model.DiffResult, a, b model.RuntimeGraph) {
	if a.Resources.PeakMemoryBytes>0 && b.Resources.PeakMemoryBytes>0 {
		ratio := float64(b.Resources.PeakMemoryBytes)/float64(a.Resources.PeakMemoryBytes)
		if ratio>=1.25 && b.Resources.PeakMemoryBytes-a.Resources.PeakMemoryBytes>=8<<20 {
			sev := model.SeverityMedium; if ratio>=2 { sev = model.SeverityHigh }
			r.Changes = append(r.Changes, model.Change{Surface:"resource", Kind:"increased", Key:"peak_memory_bytes", Before:a.Resources.PeakMemoryBytes, After:b.Resources.PeakMemoryBytes, Severity:sev, Summary:fmt.Sprintf("peak memory increased %.0f%% (%s → %s)",(ratio-1)*100,humanBytes(a.Resources.PeakMemoryBytes),humanBytes(b.Resources.PeakMemoryBytes))})
		}
	}
}

func compareLifecycle(r *model.DiffResult, a, b model.RuntimeGraph) {
	if a.Lifecycle.ShutdownMillis>0 && b.Lifecycle.ShutdownMillis>0 {
		delta := b.Lifecycle.ShutdownMillis-a.Lifecycle.ShutdownMillis
		if delta>1000 && float64(b.Lifecycle.ShutdownMillis)/math.Max(1,float64(a.Lifecycle.ShutdownMillis))>1.5 {
			sev := model.SeverityMedium; if delta>5000 { sev = model.SeverityHigh }
			r.Changes = append(r.Changes, model.Change{Surface:"lifecycle", Kind:"slower", Key:"shutdown_millis", Before:a.Lifecycle.ShutdownMillis, After:b.Lifecycle.ShutdownMillis, Severity:sev, Summary:fmt.Sprintf("graceful shutdown slowed from %.1fs to %.1fs",float64(a.Lifecycle.ShutdownMillis)/1000,float64(b.Lifecycle.ShutdownMillis)/1000)})
		}
	}
	if !a.Lifecycle.OOMKilled && b.Lifecycle.OOMKilled { r.Changes = append(r.Changes, model.Change{Surface:"lifecycle", Kind:"regression", Key:"oom_killed", After:true, Severity:model.SeverityCritical, Summary:"new version was OOM-killed during the scenario"}) }
	if a.Lifecycle.ExitCode==0 && b.Lifecycle.ExitCode!=0 { r.Changes = append(r.Changes, model.Change{Surface:"lifecycle", Kind:"regression", Key:"exit_code", Before:0, After:b.Lifecycle.ExitCode, Severity:model.SeverityCritical, Summary:fmt.Sprintf("new version exits non-zero (%d)",b.Lifecycle.ExitCode)}) }
}

func networkKey(n model.NetworkEndpoint) string { if n.Direction=="listen" { return fmt.Sprintf("%s|listen|%d",n.Protocol,n.LocalPort) }; return fmt.Sprintf("%s|%s|%s|%d",n.Protocol,n.Direction,n.RemoteAddress,n.RemotePort) }
func networkHuman(n model.NetworkEndpoint) string { if n.Direction=="listen" { return fmt.Sprintf("%s :%d",strings.ToUpper(n.Protocol),n.LocalPort) }; return fmt.Sprintf("%s %s:%d",strings.ToUpper(n.Protocol),n.RemoteAddress,n.RemotePort) }
func humanProcess(k string) string { p:=strings.SplitN(k,"|",2); if len(p)==2 && p[1]!="" { return p[0]+" ("+p[1]+")" }; return p[0] }
func set(v []string) map[string]bool { m:=map[string]bool{}; for _,x:=range v {m[x]=true}; return m }
func severityRank(s model.ChangeSeverity) int { switch s { case model.SeverityCritical:return 5; case model.SeverityHigh:return 4; case model.SeverityMedium:return 3; case model.SeverityLow:return 2; default:return 1 } }
func humanBytes(v int64) string { if v>=1<<30{return fmt.Sprintf("%.1f GiB",float64(v)/(1<<30))}; if v>=1<<20{return fmt.Sprintf("%.1f MiB",float64(v)/(1<<20))}; if v>=1<<10{return fmt.Sprintf("%.1f KiB",float64(v)/(1<<10))}; return fmt.Sprintf("%d B",v) }
