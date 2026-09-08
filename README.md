# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

Workload ABI (`wabi`) is an experimental open-source framework for discovering **operational breaking changes** between container releases. It executes two images under the same scenario, records their observed runtime behavior, computes a semantic runtime diff, and can test that delta against a target Docker Compose environment.

[![CI](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.23%2B-00ADD8.svg)](go.mod)

## Why

Software has compatibility contracts at multiple layers: APIs have schemas and semantic versioning; binaries have ABIs; container images have SBOMs, provenance, signatures, and increasingly behavioral profiles. What remains difficult to answer automatically is:

> **Will this new container release still satisfy the operational assumptions of the environment that runs the old one?**

A release can preserve its public API while changing its operational interface with the environment by:

- writing to a new filesystem path;
- opening or contacting a new network endpoint;
- spawning a new child process;
- requiring additional Linux privileges;
- increasing its minimum viable resource envelope;
- changing startup or shutdown semantics.

Workload ABI treats those surfaces as an **Operational ABI**.

## What `wabi compare` does

```text
                  identical scenario
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
          image:v1              image:v2
              │                     │
              ▼                     ▼
       RuntimeGraph A         RuntimeGraph B
              └──────────┬──────────┘
                         ▼
                  semantic diff
                         │
                         ▼
                 target constraints
                   (Compose v0.1)
                         │
                         ▼
          COMPATIBLE / DEGRADED / BREAKING
```

Current observation surfaces:

- **process** — observed process names and command lines;
- **network** — listening sockets plus inbound/outbound connections visible from `/proc/net`;
- **filesystem** — container layer mutations from `docker diff`;
- **privilege** — runtime user, effective UID/GID, Linux capabilities, seccomp and no-new-privileges state;
- **resource** — observed peak memory and CPU samples;
- **lifecycle** — readiness, graceful shutdown duration, exit code, and OOM state.

The v0.1 recorder intentionally uses the Docker CLI and container `/proc` instead of requiring a privileged eBPF sensor. This keeps the first implementation easy to run locally while preserving a clean recorder interface for future eBPF backends.

## Quick start

Requirements:

- Go 1.23+
- Docker Engine / Docker Desktop
- Docker Compose v2 when using `--target`

Build:

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make build
```

Record one image:

```bash
./bin/wabi record nginx:alpine --output nginx.json
```

Compare two releases under the same scenario:

```bash
./bin/wabi compare app:v1 app:v2 --scenario scenario.json
```

Compare and evaluate against a Compose deployment:

```bash
./bin/wabi compare app:v1 app:v2 \
  --scenario scenario.json \
  --target compose.yaml \
  --service app
```

Machine-readable output is available with `--json result.json`. Captured graphs can be retained with `--graphs-dir out/graphs`.

### Exit codes

`wabi compare` is CI-friendly:

| Code | Meaning |
| ---: | --- |
| `0` | command succeeded and configured failure threshold was not reached |
| `1` | runtime/tooling error |
| `2` | invalid CLI usage |
| `3` | `--fail-on degraded` threshold reached |
| `4` | breaking compatibility proven |

Use `--fail-on never` for exploratory runs.

## Demo: an API-compatible patch that breaks deployment

The repository contains two tiny payment API images. They expose the same HTTP endpoint, but `v2` quietly introduces a persistent write, an extra listening socket, a child shell, a much larger memory envelope, and an 8-second shutdown delay.

```bash
make demo
```

Representative output:

```text
WORKLOAD ABI
────────────────────────────────────────────────────────────────
wabi-demo/payment-api:v1 → wabi-demo/payment-api:v2

FILESYSTEM
  △ MEDIUM    new filesystem mutation A /var/lib/payment/cache

LIFECYCLE
  ! HIGH      graceful shutdown slowed from 0.3s to 8.1s

NETWORK
  △ MEDIUM    new listen network behavior: TCP :9090

PROCESS
  △ MEDIUM    new runtime process observed: sh (/bin/sh -c sleep 2)

RESOURCE
  ! HIGH      peak memory increased ...

TARGET COMPATIBILITY
────────────────────────────────────────────────────────────────
Environment: examples/payment-api/compose.yaml (service: payment-api)
  ✗ FILESYSTEM the new release introduces a filesystem mutation outside configured tmpfs mounts
  ✗ RESOURCE   observed peak memory exceeds the Compose memory limit
  ! LIFECYCLE  the new release takes longer to stop than the configured grace period

RUNTIME COMPATIBILITY: BREAKING
```

This is the central hypothesis of the project: **compatibility is a relationship between old behavior, new behavior, and the target environment.** A behavior change is not inherently breaking everywhere.

## Scenario format

Scenarios make the comparison controlled and reproducible. Both releases receive the exact same environment, published ports, command, probes, and timing.

```json
{
  "name": "api-smoke",
  "observation_ms": 3000,
  "sample_interval_ms": 300,
  "shutdown_timeout_ms": 15000,
  "probes": [
    {
      "port": 8080,
      "method": "GET",
      "path": "/health",
      "expect_status": 200,
      "repeat": 2
    }
  ]
}
```

See [`schemas/scenario.schema.json`](schemas/scenario.schema.json) and [`docs/methodology.md`](docs/methodology.md).

## RuntimeGraph

Each recording becomes a normalized, versioned artifact:

```json
{
  "schema_version": "wabi.dev/v1alpha1",
  "image": {
    "reference": "app:v2",
    "digest": "sha256:..."
  },
  "scenario": "api-smoke",
  "processes": [],
  "network": [],
  "filesystem": [],
  "privileges": {},
  "resources": {},
  "lifecycle": {}
}
```

RuntimeGraph is deliberately independent of Docker's raw event format. Future recorders can produce the same graph from eBPF, Falco, Tracee, Tetragon, Kubernetes, or another runtime source.

## How this differs from adjacent work

Workload ABI is intended to complement, not replace, existing artifact and behavioral standards:

- **SBOM / provenance / VEX** explain what was built, from what, and known artifact risk.
- **behavior profiles / SBoB-style artifacts** describe or prescribe what software is expected to do.
- **runtime security tools** detect suspicious or policy-violating activity.
- **Workload ABI** asks a differential compatibility question: *what operational behavior changed between two releases, and does that delta conflict with this target environment?*

The project does not claim that behavioral profiling itself is novel. The research hypothesis is the combination of **controlled differential execution + normalized Operational ABI + environment-aware compatibility solving**.

## Design principles

1. **Observed, not guessed.** Runtime evidence is the primary input.
2. **Differential.** The meaningful unit is the behavioral delta between releases.
3. **Environment-aware.** A change becomes breaking only in context.
4. **Vendor-neutral.** Docker is the first execution substrate, not the data model.
5. **Explainable.** Every incompatibility must point to evidence and a target constraint.
6. **Deterministic where possible.** Same scenario, bounded observation window, normalized artifacts.
7. **No LLM dependency.** The core must remain useful and testable without AI.

## Roadmap

### v0.1 — Docker differential runtime

- [x] `wabi record`
- [x] `wabi diff`
- [x] `wabi compare`
- [x] RuntimeGraph v1alpha1
- [x] process/network/filesystem/privilege/resource/lifecycle surfaces
- [x] Docker Compose compatibility solver
- [x] JSON reports and CI-friendly exit codes
- [x] end-to-end demo and GitHub Actions smoke test

### v0.2 — Better runtime fidelity

- [ ] Linux eBPF recorder
- [ ] DNS-aware network identities instead of IP-only observations
- [ ] workload-specific path normalization and noise suppression
- [ ] explicit minimum resource-envelope experiments
- [ ] repeated-run confidence / flake scoring

### v0.3 — Cloud-native targets

- [ ] Kubernetes Deployment/Pod target solver
- [ ] Helm-rendered manifest support
- [ ] NetworkPolicy, SecurityContext and resource-limit conflicts
- [ ] rollout/liveness/readiness compatibility

### v0.4 — Supply-chain artifact

- [ ] OCI Runtime Compatibility Attestation
- [ ] Sigstore signing and verification
- [ ] policy gates for GitHub Actions / CI systems

See [`docs/roadmap.md`](docs/roadmap.md) for the longer-term research direction.

## Status

**Experimental / v0.1-alpha.** Workload ABI is a research-oriented project. Runtime observation is necessarily incomplete, and absence of an observed behavior is not proof that the behavior is impossible. Read the [methodology and limitations](docs/methodology.md) before using results as a release gate.

## Contributing

Contributions, adversarial examples, portability fixes, new recorders, and critiques of the compatibility model are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0.
