package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
)

//go:embed index.html
var indexHTML string

type Data struct {
	Matrix         matrix.Artifact `json:"matrix"`
	GraphDiff      *graph.Diff     `json:"graph_diff,omitempty"`
	BaselineGraph  *graph.Artifact `json:"baseline_graph,omitempty"`
	CandidateGraph *graph.Artifact `json:"candidate_graph,omitempty"`
}

func Load(matrixFile, baselineGraphFile, candidateGraphFile string) (Data, error) {
	artifact, err := matrix.LoadArtifact(matrixFile)
	if err != nil {
		return Data{}, fmt.Errorf("load matrix: %w", err)
	}
	data := Data{Matrix: artifact}
	if baselineGraphFile == "" && candidateGraphFile == "" {
		return data, nil
	}
	if baselineGraphFile == "" || candidateGraphFile == "" {
		return Data{}, fmt.Errorf("baseline and candidate graph files must be supplied together")
	}

	baseGraph, err := graph.Load(baselineGraphFile)
	if err != nil {
		return Data{}, fmt.Errorf("load baseline graph: %w", err)
	}
	candidateGraph, err := graph.Load(candidateGraphFile)
	if err != nil {
		return Data{}, fmt.Errorf("load candidate graph: %w", err)
	}
	if baseGraph.SnapshotFingerprint != artifact.BaselineFingerprint {
		return Data{}, fmt.Errorf("baseline graph is bound to snapshot %s, matrix expects %s", baseGraph.SnapshotFingerprint, artifact.BaselineFingerprint)
	}
	if candidateGraph.SnapshotFingerprint != artifact.CandidateFingerprint {
		return Data{}, fmt.Errorf("candidate graph is bound to snapshot %s, matrix expects %s", candidateGraph.SnapshotFingerprint, artifact.CandidateFingerprint)
	}
	if baseGraph.Scenario != candidateGraph.Scenario || baseGraph.Scenario != artifact.Scenario {
		return Data{}, fmt.Errorf("graph scenario does not match matrix scenario")
	}

	diff := graph.Compare(baseGraph, candidateGraph)
	data.BaselineGraph = &baseGraph
	data.CandidateGraph = &candidateGraph
	data.GraphDiff = &diff
	return data, nil
}

func Render(data Data) ([]byte, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encode dashboard data: %w", err)
	}
	const marker = "__WABI_DASHBOARD_DATA__"
	if !strings.Contains(indexHTML, marker) {
		return nil, fmt.Errorf("dashboard template data marker is missing")
	}
	return []byte(strings.Replace(indexHTML, marker, string(encoded), 1)), nil
}

func Export(path string, data Data) error {
	page, err := Render(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, page, 0o644); err != nil {
		return fmt.Errorf("write dashboard: %w", err)
	}
	return nil
}

func Handler(data Data) (http.Handler, error) {
	page, err := Render(data)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(page)
	})
	return mux, nil
}

func ValidateListenAddress(address string, allowRemote bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if allowRemote {
		return nil
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing non-loopback dashboard bind %q without --allow-remote", address)
}

func Serve(ctx context.Context, address string, allowRemote bool, data Data) error {
	if err := ValidateListenAddress(address, allowRemote); err != nil {
		return err
	}
	handler, err := Handler(data)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	close(done)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve dashboard: %w", err)
	}
	return nil
}
