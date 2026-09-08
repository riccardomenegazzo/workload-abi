package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/dockercli"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

type ComposeProject struct { Services map[string]map[string]any `json:"services"` }

func EvaluateCompose(ctx context.Context, docker *dockercli.Client, path, service string, oldGraph, newGraph model.RuntimeGraph, d model.DiffResult) (model.CompatibilityResult, error) {
	out, err := docker.Run(ctx, "compose", "-f", path, "config", "--format", "json")
	if err != nil { return model.CompatibilityResult{}, fmt.Errorf("normalize compose file: %w",err) }
	var p ComposeProject
	if err:=json.Unmarshal([]byte(out),&p);err!=nil{return model.CompatibilityResult{},fmt.Errorf("decode normalized compose config: %w",err)}
	return EvaluateComposeConfig(path,service,p,oldGraph,newGraph,d)
}

func EvaluateComposeConfig(path, service string, p ComposeProject, oldGraph,newGraph model.RuntimeGraph,d model.DiffResult)(model.CompatibilityResult,error){
	if len(p.Services)==0{return model.CompatibilityResult{},fmt.Errorf("compose project has no services")}
	if service=="" { service=pickService(p,newGraph.Image.Reference) }
	cfg,ok:=p.Services[service]; if !ok { return model.CompatibilityResult{},fmt.Errorf("service %q not found in compose project",service) }
	r:=model.CompatibilityResult{Status:model.StatusCompatible,Environment:path,Service:service}
	readOnly,_:=cfg["read_only"].(bool)
	tmpfs:=stringSlice(cfg["tmpfs"])
	if readOnly {
		for _,ch:=range d.Changes {
			if ch.Surface!="filesystem"||ch.Kind!="added"{continue}
			f,ok:=changeFile(ch.After);if !ok||f.Kind=="D"{continue}
			if !pathAllowedByTmpfs(f.Path,tmpfs) { r.Conflicts=append(r.Conflicts,model.Conflict{Surface:"filesystem",Constraint:"read_only: true",Observed:f.Kind+" "+f.Path,Severity:model.SeverityCritical,Explanation:"the new release introduces a filesystem mutation outside configured tmpfs mounts"}) }
		}
	}
	if limit,ok:=parseMemLimit(cfg["mem_limit"]);ok&&limit>0&&newGraph.Resources.PeakMemoryBytes>limit { r.Conflicts=append(r.Conflicts,model.Conflict{Surface:"resource",Constraint:"mem_limit: "+humanBytes(limit),Observed:"peak memory: "+humanBytes(newGraph.Resources.PeakMemoryBytes),Severity:model.SeverityCritical,Explanation:"observed peak memory exceeds the Compose memory limit"}) }
	if grace,ok:=parseDuration(cfg["stop_grace_period"]);ok&&grace>0&&time.Duration(newGraph.Lifecycle.ShutdownMillis)*time.Millisecond>grace { r.Conflicts=append(r.Conflicts,model.Conflict{Surface:"lifecycle",Constraint:"stop_grace_period: "+grace.String(),Observed:fmt.Sprintf("shutdown: %.1fs",float64(newGraph.Lifecycle.ShutdownMillis)/1000),Severity:model.SeverityHigh,Explanation:"the new release takes longer to stop than the configured grace period and may be force-killed"}) }
	if nm,_:=cfg["network_mode"].(string); strings.EqualFold(nm,"none") { for _,ch:=range d.Changes { if ch.Surface=="network"&&ch.Kind=="added"&&strings.Contains(ch.Key,"|outbound|") { r.Conflicts=append(r.Conflicts,model.Conflict{Surface:"network",Constraint:"network_mode: none",Observed:ch.Summary,Severity:model.SeverityCritical,Explanation:"the new release introduces outbound network behavior while networking is disabled"}) } } }
	capDrop:=upperSet(stringSlice(cfg["cap_drop"])); if capDrop["ALL"] { oldCaps:=upperSet(oldGraph.Privileges.EffectiveCaps); for _,c:=range newGraph.Privileges.EffectiveCaps { if !oldCaps[strings.ToUpper(c)] { r.Conflicts=append(r.Conflicts,model.Conflict{Surface:"privilege",Constraint:"cap_drop: [ALL]",Observed:"new effective capability: "+c,Severity:model.SeverityCritical,Explanation:"the new release observed an additional Linux capability while the target drops all capabilities"}) } } }
	for _,c:=range r.Conflicts { if c.Severity==model.SeverityCritical||c.Severity==model.SeverityHigh { r.Status=model.StatusBreaking; break }; if r.Status==model.StatusCompatible {r.Status=model.StatusDegraded} }
	if len(r.Conflicts)==0 && (d.Summary.High>0||d.Summary.Critical>0) { r.Status=model.StatusDegraded; r.Notes=append(r.Notes,"high-severity runtime changes were observed, but no direct conflict with modeled Compose constraints was proven") }
	return r,nil
}

func pickService(p ComposeProject,image string)string{ if len(p.Services)==1{for k:=range p.Services{return k}}; for name,cfg:=range p.Services { if v,_:=cfg["image"].(string); v==image {return name} }; names:=make([]string,0,len(p.Services));for n:=range p.Services{names=append(names,n)}; for i:=0;i<len(names);i++{for j:=i+1;j<len(names);j++{if names[j]<names[i]{names[i],names[j]=names[j],names[i]}}}; if len(names)>0{return names[0]};return "" }
func stringSlice(v any)[]string{ var out []string; switch x:=v.(type){case []any:for _,e:=range x{if s,ok:=e.(string);ok{out=append(out,s)}};case []string:out=append(out,x...);case string:out=append(out,x)};return out }
func upperSet(v []string)map[string]bool{m:=map[string]bool{};for _,s:=range v{m[strings.ToUpper(s)]=true};return m}
func pathAllowedByTmpfs(path string,tmpfs []string)bool{clean:=filepath.Clean(path);for _,entry:=range tmpfs{mount:=strings.SplitN(entry,":",2)[0];mount=filepath.Clean(mount);if clean==mount||strings.HasPrefix(clean,mount+string(filepath.Separator)){return true}};return false}
func changeFile(v any)(model.FileMutation,bool){m,ok:=v.(model.FileMutation);if ok{return m,true}; raw,err:=json.Marshal(v);if err!=nil{return model.FileMutation{},false};if json.Unmarshal(raw,&m)!=nil{return model.FileMutation{},false};return m,true}
func parseMemLimit(v any)(int64,bool){switch x:=v.(type){case float64:return int64(x),true;case json.Number:n,e:=x.Int64();return n,e==nil;case string:n,e:=parseBytes(x);return n,e==nil};return 0,false}
func parseBytes(s string)(int64,error){s=strings.TrimSpace(strings.ToLower(s));for _,u:=range []struct{s string;m int64}{{"gib",1<<30},{"gb",1e9},{"g",1<<30},{"mib",1<<20},{"mb",1e6},{"m",1<<20},{"kib",1<<10},{"kb",1e3},{"k",1<<10},{"b",1}}{if strings.HasSuffix(s,u.s){n,e:=strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s,u.s)),64);if e!=nil{return 0,e};return int64(n*float64(u.m)),nil}};n,e:=strconv.ParseInt(s,10,64);return n,e}
func parseDuration(v any)(time.Duration,bool){switch x:=v.(type){case string:if x==""{return 0,false};d,e:=time.ParseDuration(x);return d,e==nil;case float64:return time.Duration(int64(x)),true;case json.Number:n,e:=x.Int64();return time.Duration(n),e==nil};return 0,false}
func humanBytes(v int64)string{if v>=1<<30{return fmt.Sprintf("%.1fGiB",float64(v)/(1<<30))};if v>=1<<20{return fmt.Sprintf("%.1fMiB",float64(v)/(1<<20))};if v>=1<<10{return fmt.Sprintf("%.1fKiB",float64(v)/(1<<10))};return fmt.Sprintf("%dB",v)}
