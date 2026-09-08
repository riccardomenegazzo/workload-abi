# Architecture

Workload ABI separates observation from compatibility reasoning so the project can evolve beyond Docker without changing its core model.

```text
Scenario
   │
   ├──────────────┐
   ▼              ▼
image A        image B
   │              │
Recorder        Recorder
   │              │
   ▼              ▼
RuntimeGraph A  RuntimeGraph B
   └──────┬───────┘
          ▼
   Semantic Differ
          │
          ▼
   Operational Delta
          │
          ├──────────────► text / JSON report
          │
          ▼
  Compatibility Solver
          │
          ▼
 Target-specific conflicts
```

## Packages

- `internal/dockercli`: thin adapter around the Docker CLI.
- `internal/observe`: Docker-based runtime observation and normalization.
- `internal/model`: versioned RuntimeGraph, diff, and compatibility models.
- `internal/diff`: semantic changes between two normalized graphs.
- `internal/compat`: target-environment solvers; Docker Compose is first.
- `internal/scenario`: controlled stimulus format shared by both executions.
- `internal/report`: human-oriented output.
- `cmd/wabi`: stable CLI boundary.

## Why a normalized graph

Raw runtime events are recorder-specific and noisy. Workload ABI intentionally loses some event detail in exchange for a stable semantic layer. The graph records operational facts such as "process X existed", "port Y listened", "path Z changed", and measured lifecycle/resource properties.

A future eBPF recorder should emit the same `RuntimeGraph` schema. Compatibility logic must not need to know whether an observation came from Docker `/proc`, Falco, Tracee, Tetragon, or another source.

## Trust boundary

The recorder executes user-supplied container images. Docker is treated as the isolation boundary. `wabi` does not mount the Docker socket into the workload and does not intentionally inject credentials. Scenarios should use synthetic data and disposable environments.
