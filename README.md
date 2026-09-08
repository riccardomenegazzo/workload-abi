# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

Workload ABI (`wabi`) is an experimental, vendor-neutral tool for discovering **operational breaking changes** between container releases.

It executes two container images under equivalent conditions, captures their observable runtime behavior, normalizes that behavior into a workload snapshot, and reports the semantic differences that may matter to production.

## Why

Software already has contracts for source APIs, binary ABIs, package dependencies, SBOMs, signatures, and provenance. What is usually missing is a compatibility model for the *operational interface* between a workload and its environment.

A patch release can keep the same HTTP API and still:

- spawn a new helper process;
- mutate new filesystem paths;
- expose a new port;
- change its runtime user or entrypoint;
- require additional capabilities;
- fail under the same startup workflow;
- take materially longer to terminate.

Those are operational changes. `wabi` makes them visible.

## Current v0.1

The first implementation intentionally starts small and measurable:

- Docker image orchestration through the local Docker Engine;
- process snapshot collection with `docker top`;
- filesystem mutation collection with `docker diff`;
- image/runtime configuration capture with `docker inspect`;
- CPU/memory/network/block-I/O snapshot collection with `docker stats`;
- startup/shutdown lifecycle timings;
- deterministic semantic diffing;
- human-readable and JSON reports;
- CI-safe exit codes.

This is **not yet** a complete eBPF-based behavioral ABI. Network flows, syscall requirements, kernel interactions, deterministic workload stimuli, Compose/Kubernetes environment solving, and OCI attestations are roadmap items.

## Install

Requirements:

- Go 1.23+
- Docker Engine / Docker Desktop

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make build
```

The CLI is built at `./bin/wabi`.

## Compare two releases

```bash
./bin/wabi compare nginx:1.26 nginx:1.27
```

Increase the observation window when needed:

```bash
./bin/wabi compare --observe 5s app:v1 app:v2
```

Machine-readable output:

```bash
./bin/wabi compare --json app:v1 app:v2
```

Fail CI whenever *any* operational change is found:

```bash
./bin/wabi compare --fail-on-change app:v1 app:v2
```

Exit codes:

- `0`: no fatal error;
- `3`: changes found with `--fail-on-change`;
- `4`: a breaking runtime regression was detected;
- `1/2`: execution or usage error.

## Record one workload snapshot

```bash
./bin/wabi record --observe 3s nginx:1.27 > snapshot.json
```

The snapshot is designed to become a stable, versioned input for future compatibility analysis.

## Example report

```text
WORKLOAD ABI
================================================================
payment-api:1.8.3 -> payment-api:1.8.4

PROCESS
  + [WARNING] new process observed: curl telemetry.example

FILESYSTEM
  + [WARNING] new filesystem mutation observed: A /var/lib/payment/cache

IMAGE-CONFIG
  ~ [WARNING] user changed
      before: 1000
      after:  0

----------------------------------------------------------------
RUNTIME COMPATIBILITY: CHANGED
```

## Design principles

1. **Observe, do not guess.** Runtime evidence is first-class.
2. **Same experiment, two releases.** Comparison is meaningful only under equivalent conditions.
3. **Semantic differences, not log diffs.** Raw events are normalized before comparison.
4. **Vendor neutral.** Docker is the first execution backend, not the project boundary.
5. **Explainable verdicts.** A compatibility result must point to concrete evidence.
6. **Progressive depth.** v0.1 uses portable Docker primitives; later recorders can add eBPF/Falco/Tracee without changing the model.

## Architecture

```text
             equivalent observation workflow
                        |
          +-------------+-------------+
          |                           |
      image:v1                      image:v2
          |                           |
          v                           v
   runtime snapshot A          runtime snapshot B
          |                           |
          +-------------+-------------+
                        |
                  semantic diff
                        |
                        v
              compatibility verdict
```

See [`docs/architecture.md`](docs/architecture.md) for the internal model and roadmap.

## Roadmap

### v0.2 — deterministic scenarios

- repeatable HTTP/CLI stimuli;
- environment variables, mounts, and command overrides;
- normalization to eliminate incidental runtime noise.

### v0.3 — target environment solver

```bash
wabi compare app:v1 app:v2 --target compose.yaml
```

The goal is to move from **"what changed?"** to **"will this change violate the target environment?"**

### v0.4 — deep runtime recorder

- eBPF process/file/network observation;
- syscall and capability requirements;
- DNS and outbound dependency graph;
- causal runtime graph.

### v0.5 — supply-chain artifact

- OCI-linked runtime compatibility attestation;
- Sigstore signing;
- CI/CD policy integration.

### v1.0 — Operational ABI

A stable compatibility model for the operational interface between a workload release and its execution environment.

## Status

Workload ABI is an early-stage research/engineering project. The v0.1 semantics will evolve as the recorder becomes deeper and deterministic scenario execution is introduced.

## License

Apache License 2.0.
