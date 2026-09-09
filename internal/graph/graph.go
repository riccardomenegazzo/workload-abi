package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	SchemaVersion     = "wabi.graph/v1alpha1"
	DiffSchemaVersion = "wabi.graph-diff/v1alpha1"
	rootNodeID        = "workload:root"
)

type Node struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type Edge struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
}

func (e Edge) Key() string {
	return e.From + "\x00" + e.Relation + "\x00" + e.To
}

type Artifact struct {
	SchemaVersion       string `json:"schema_version"`
	SnapshotSchema      string `json:"snapshot_schema"`
	SnapshotFingerprint string `json:"snapshot_fingerprint"`
	Fingerprint         string `json:"fingerprint"`
	Image               string `json:"image,omitempty"`
	Scenario            string `json:"scenario,omitempty"`
	Nodes               []Node `json:"nodes"`
	Edges               []Edge `json:"edges"`
}

type EdgeChange struct {
	Kind string `json:"kind"`
	Edge Edge   `json:"edge"`
}

type Explanation struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Edges   []Edge `json:"edges"`
}

type Diff struct {
	SchemaVersion        string        `json:"schema_version"`
	BaselineFingerprint  string        `json:"baseline_fingerprint"`
	CandidateFingerprint string        `json:"candidate_fingerprint"`
	Verdict              string        `json:"verdict"`
	Changes              []EdgeChange  `json:"changes"`
	Explanations         []Explanation `json:"explanations,omitempty"`
}

func Build(snapshot model.Snapshot) Artifact {
	g := Artifact{
		SchemaVersion:       SchemaVersion,
		SnapshotSchema:      snapshot.SchemaVersion,
		SnapshotFingerprint: snapshot.Fingerprint,
		Image:               snapshot.Image,
		Scenario:            snapshot.Scenario,
	}

	nodes := map[string]Node{
		rootNodeID: {ID: rootNodeID, Kind: "workload", Label: "workload"},
	}
	edges := map[string]Edge{}

	ensureProcess := func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return rootNodeID
		}
		id := "process:" + name
		nodes[id] = Node{ID: id, Kind: "process", Label: name}
		return id
	}
	ensureResource := func(kind, label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			return ""
		}
		id := kind + ":" + label
		nodes[id] = Node{ID: id, Kind: kind, Label: label}
		return id
	}
	addEdge := func(from, relation, to string) {
		if from == "" || relation == "" || to == "" || from == to {
			return
		}
		e := Edge{From: from, Relation: relation, To: to}
		edges[e.Key()] = e
	}

	for _, event := range snapshot.RuntimeEvents {
		process := ensureProcess(event.Process)
		parent := ensureProcess(event.ParentProcess)
		switch event.Category {
		case "process":
			if event.Process == "" {
				continue
			}
			relation := event.Operation
			if relation == "" {
				relation = "exec"
			}
			if event.ParentProcess == "" {
				parent = rootNodeID
			}
			addEdge(parent, relation, process)
		case "file":
			resource := ensureResource("file", event.Target)
			addEdge(process, event.Operation, resource)
		case "network":
			kind := "endpoint"
			if event.Operation == "dns" {
				kind = "domain"
			}
			resource := ensureResource(kind, event.Target)
			relation := event.Operation
			if event.Direction != "" {
				relation += ":" + event.Direction
			}
			addEdge(process, relation, resource)
		case "syscall":
			resource := ensureResource("syscall", event.Operation)
			addEdge(process, "uses", resource)
		default:
			resource := ensureResource(event.Category, event.Target)
			addEdge(process, event.Operation, resource)
		}
	}

	g.Nodes = make([]Node, 0, len(nodes))
	for _, node := range nodes {
		g.Nodes = append(g.Nodes, node)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	g.Edges = make([]Edge, 0, len(edges))
	for _, edge := range edges {
		g.Edges = append(g.Edges, edge)
	}
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].Key() < g.Edges[j].Key() })
	g.Fingerprint = Fingerprint(g)
	return g
}

func Fingerprint(g Artifact) string {
	payload := struct {
		SchemaVersion       string `json:"schema_version"`
		SnapshotFingerprint string `json:"snapshot_fingerprint"`
		Nodes               []Node `json:"nodes"`
		Edges               []Edge `json:"edges"`
	}{
		SchemaVersion:       SchemaVersion,
		SnapshotFingerprint: g.SnapshotFingerprint,
		Nodes:               append([]Node(nil), g.Nodes...),
		Edges:               append([]Edge(nil), g.Edges...),
	}
	sort.Slice(payload.Nodes, func(i, j int) bool { return payload.Nodes[i].ID < payload.Nodes[j].ID })
	sort.Slice(payload.Edges, func(i, j int) bool { return payload.Edges[i].Key() < payload.Edges[j].Key() })
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Load(path string) (Artifact, error) {
	var g Artifact
	data, err := os.ReadFile(path)
	if err != nil {
		return g, fmt.Errorf("read graph: %w", err)
	}
	if err := json.Unmarshal(data, &g); err != nil {
		return g, fmt.Errorf("parse graph: %w", err)
	}
	if g.SchemaVersion != SchemaVersion {
		return g, fmt.Errorf("unsupported graph schema %q", g.SchemaVersion)
	}
	if g.SnapshotFingerprint == "" {
		return g, fmt.Errorf("graph snapshot_fingerprint is required")
	}
	computed := Fingerprint(g)
	if g.Fingerprint == "" || g.Fingerprint != computed {
		return g, fmt.Errorf("graph fingerprint mismatch: stored %s computed %s", g.Fingerprint, computed)
	}
	return g, nil
}

func Compare(base, candidate Artifact) Diff {
	d := Diff{
		SchemaVersion:        DiffSchemaVersion,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Verdict:              "COMPATIBLE",
	}
	before := edgeMap(base.Edges)
	after := edgeMap(candidate.Edges)
	for key, edge := range after {
		if _, ok := before[key]; !ok {
			d.Changes = append(d.Changes, EdgeChange{Kind: "added", Edge: edge})
		}
	}
	for key, edge := range before {
		if _, ok := after[key]; !ok {
			d.Changes = append(d.Changes, EdgeChange{Kind: "removed", Edge: edge})
		}
	}
	sort.Slice(d.Changes, func(i, j int) bool {
		if d.Changes[i].Kind == d.Changes[j].Kind {
			return d.Changes[i].Edge.Key() < d.Changes[j].Edge.Key()
		}
		return d.Changes[i].Kind < d.Changes[j].Kind
	})
	if len(d.Changes) > 0 {
		d.Verdict = "CHANGED"
	}
	d.Explanations = explain(candidate, d.Changes)
	return d
}

func edgeMap(edges []Edge) map[string]Edge {
	out := make(map[string]Edge, len(edges))
	for _, edge := range edges {
		out[edge.Key()] = edge
	}
	return out
}

func nodeMap(nodes []Node) map[string]Node {
	out := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func explain(candidate Artifact, changes []EdgeChange) []Explanation {
	nodes := nodeMap(candidate.Nodes)
	var added []Edge
	for _, change := range changes {
		if change.Kind == "added" {
			added = append(added, change.Edge)
		}
	}

	covered := map[string]struct{}{}
	var out []Explanation
	for _, edge := range added {
		if edge.Relation != "spawn" && edge.Relation != "exec" {
			continue
		}
		child := nodes[edge.To]
		if child.Kind != "process" {
			continue
		}
		chain := []Edge{edge}
		var clauses []string
		for _, candidateEdge := range added {
			if candidateEdge.From != edge.To {
				continue
			}
			target := nodes[candidateEdge.To]
			if target.Kind != "file" && target.Kind != "endpoint" && target.Kind != "domain" {
				continue
			}
			chain = append(chain, candidateEdge)
			clauses = append(clauses, relationPhrase(candidateEdge.Relation, target.Label))
			covered[candidateEdge.Key()] = struct{}{}
		}
		parent := nodes[edge.From]
		verb := "spawned by"
		if edge.Relation == "exec" {
			verb = "executed by"
		}
		summary := fmt.Sprintf("New process %s is %s %s", child.Label, verb, parent.Label)
		if len(clauses) > 0 {
			summary += " and " + joinClauses(clauses)
		}
		summary += "."
		covered[edge.Key()] = struct{}{}
		out = append(out, Explanation{Kind: "new-process-chain", Summary: summary, Edges: chain})
	}

	for _, edge := range added {
		if _, ok := covered[edge.Key()]; ok {
			continue
		}
		from := nodes[edge.From]
		to := nodes[edge.To]
		if from.Kind != "process" || (to.Kind != "file" && to.Kind != "endpoint" && to.Kind != "domain") {
			continue
		}
		summary := fmt.Sprintf("Process %s now %s.", from.Label, relationPhrase(edge.Relation, to.Label))
		out = append(out, Explanation{Kind: "new-dependency", Summary: summary, Edges: []Edge{edge}})
		covered[edge.Key()] = struct{}{}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Summary < out[j].Summary })
	return out
}

func relationPhrase(relation, target string) string {
	switch relation {
	case "read":
		return "reads " + target
	case "write":
		return "writes " + target
	case "open":
		return "opens " + target
	case "connect", "connect:outbound":
		return "connects to " + target
	case "send", "send:outbound":
		return "sends data to " + target
	case "dns", "dns:outbound":
		return "resolves " + target
	case "accept", "accept:inbound":
		return "accepts traffic from " + target
	case "listen", "listen:inbound":
		return "listens on " + target
	default:
		return relation + "s " + target
	}
}

func joinClauses(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	if len(clauses) == 1 {
		return clauses[0]
	}
	if len(clauses) == 2 {
		return clauses[0] + " and " + clauses[1]
	}
	return strings.Join(clauses[:len(clauses)-1], ", ") + ", and " + clauses[len(clauses)-1]
}
