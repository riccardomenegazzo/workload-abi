# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

[![CI](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml)
[![Environment Matrix](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/environment-matrix.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/environment-matrix.yml)
[![Native eBPF](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/native-ebpf.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/native-ebpf.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Workload ABI (`wabi`) discovers operational breaking changes between container releases and proves where a candidate can actually run.**

It executes equivalent workload experiments, records normalized runtime evidence, accepts deep evidence from independent sensors, derives causal runtime graphs, evaluates production constraints, and produces a fingerprinted **Environment Compatibility Matrix** with an embedded deployability dashboard.

```text
                         SAME EXPERIMENT
                               │
                  ┌────────────┴────────────┐
                  │                         │
              image:v1                 image:v2
                  │                         │
                  ▼                         ▼
             Docker record            Docker record
                  │                         │
           snapshot / fingerprint   snapshot / fingerprint
                  │                         │
          optional deep evidence   optional deep evidence
       Falco / Tracee / native / generic providers
                  │                         │
                  └────────────┬────────────┘
                               ▼
                    semantic diff + causal graph
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
           Compose        Kubernetes          ECS
                              │
                         NetworkPolicy
              │                │                │
              └─────────┬──────┴──────┬─────────┘
                        ▼             ▼
                     seccomp        policy
                        │             │
                        └──────┬──────┘
                               ▼
                ENVIRONMENT COMPATIBILITY MATRIX
                               │
             ┌─────────────────┼─────────────────┐
             ▼                 ▼                 ▼
        COMPATIBLE          CHANGED          BREAKING
        deployable      deployable/review      blocked
                               │
                               ▼
              Dashboard / JSON / SARIF / in-toto
```

This is **not** an SBOM scanner, vulnerability scanner, attack simulator, or raw log differ.

It answers a different question:

> **Did this release change its operational interface, why did it change, and in which environments is that change deployable?**

---

## Why this exists

A release can keep the same API, pass every unit test, build successfully, and still break production because it now:

- writes to a path that is read-only in production;
- opens a new listener;
- contacts a new external dependency blocked by NetworkPolicy;
- starts a helper process;
- reads a credential it never touched before;
- requires a syscall blocked by seccomp;
- needs root or an additional Linux capability;
- exceeds a memory limit;
- takes longer than the deployment shutdown grace period;
- behaves differently under the same runtime stimulus.

Traditional SemVer describes source/API compatibility. Workload ABI treats runtime assumptions as an **Operational ABI**.

The core rule is:

> **A difference is not automatically a breaking change.**
>
> Evidence describes what changed. Environment constraints determine whether that change is deployable.

---

## The 60-second path

### 1. Record equivalent releases

```bash
wabi record --scenario scenario.json app:1.8.3 > baseline.json
wabi record --scenario scenario.json app:1.8.4 > candidate.json
```

### 2. Optionally enrich with deep runtime evidence

Falco:

```bash
wabi enrich \
  --snapshot candidate.json \
  --events falco.jsonl \
  --format falco \
  --output candidate.deep.json
```

Tracee:

```bash
wabi enrich \
  --snapshot candidate.json \
  --events tracee.jsonl \
  --format tracee \
  --output candidate.deep.json
```

Native Linux eBPF:

```bash
sudo wabi-native record \
  --duration 10s \
  --output native-events.json \
  --stats-output native-stats.json

wabi enrich \
  --snapshot candidate.json \
  --events native-events.json \
  --format generic \
  --output candidate.deep.json
```

### 3. Prove the release against several environments

```bash
wabi matrix \
  --config environments.json \
  --output matrix.json \
  baseline.deep.json candidate.deep.json
```

Example result:

```text
WORKLOAD ABI — ENVIRONMENT COMPATIBILITY MATRIX
================================================================
app:1.8.3 -> app:1.8.4

ENVIRONMENT                 VERDICT      BLOCKERS
----------------------------------------------------------------
developer-compose           CHANGED      -
staging-kubernetes          BREAKING     runtime-network
production-ecs              BREAKING     filesystem
hardened-runtime            BREAKING     syscall

----------------------------------------------------------------
MATRIX VERDICT: BREAKING
compatible=0 changed=1 breaking=3
```

`CHANGED` means the release differs but no configured constraint proves it cannot run. It is **deployable with review**. Only `BREAKING` is blocked.

### 4. Open the dashboard

```bash
wabi dashboard --matrix matrix.json
```

Default URL:

```text
http://127.0.0.1:8787
```

Or export one portable HTML artifact:

```bash
wabi dashboard \
  --matrix matrix.json \
  --export dashboard.html
```

No Node, npm, CDN, database, or external backend is required.

---

## Deployability Dashboard

The dashboard is embedded in the Go binary and renders only verified artifacts.

It shows:

- baseline and candidate workload identity;
- Operational ABI fingerprints;
- deployable vs blocked environment counts;
- per-environment target and policy identity;
- blocker surfaces (`filesystem`, `runtime-network`, `syscall`, `privilege`, `resource`, `lifecycle`, ...);
- expandable normalized evidence;
- search and deployable/blocked filtering;
- blocker concentration;
- artifact integrity bindings;
- optional causal explanations;
- matrix JSON export.

With causal graphs:

```bash
wabi graph --output baseline.graph.json baseline.deep.json
wabi graph --output candidate.graph.json candidate.deep.json

wabi dashboard \
  --matrix matrix.json \
  --baseline-graph baseline.graph.json \
  --candidate-graph candidate.graph.json
```

The dashboard refuses a graph if its `snapshot_fingerprint` does not match the exact snapshot fingerprint stored in the matrix. Causal visualization is therefore linked to the same evidence used for deployability decisions.

A typical explanation can become:

```text
New process /usr/bin/helper is executed by /app,
reads /var/run/secrets/token and connects to api.vendor.com:443.
```

See [`docs/deployability-dashboard.md`](docs/deployability-dashboard.md).

---

## Environment Compatibility Matrix

The matrix is a derived, independently versioned artifact:

```text
wabi.matrix/v1alpha1
```

Configuration is also versioned:

```text
wabi.matrix-config/v1alpha1
```

Example:

```json
{
  "schema_version": "wabi.matrix-config/v1alpha1",
  "environments": [
    {
      "name": "developer-compose",
      "target": "compose.yaml",
      "target_kind": "compose",
      "service": "api"
    },
    {
      "name": "staging-kubernetes",
      "target": "deployment.json",
      "target_kind": "kubernetes",
      "container": "api",
      "network_policy": "egress.json"
    },
    {
      "name": "production-ecs",
      "target": "task-definition.json",
      "target_kind": "ecs",
      "container": "api"
    },
    {
      "name": "hardened-runtime",
      "seccomp_profile": "seccomp.json"
    }
  ]
}
```

Paths are resolved relative to the matrix config file, so an environment definition can be committed and moved with the repository.

Each matrix artifact contains:

- baseline/candidate image identity;
- baseline/candidate Operational ABI fingerprints;
- scenario identity;
- complete per-environment changes;
- target/policy identity;
- compatible/changed/breaking summary;
- deterministic matrix fingerprint.

Verify a persisted matrix without rerunning workloads:

```bash
wabi verify-matrix matrix.json
```

Tampering with a verdict, blocker, environment, or evidence binding invalidates the fingerprint.

---

## Evidence model

### Equivalent experiments

Docker is the reference execution backend. A scenario can apply identical inputs to baseline and candidate:

- environment variables;
- command override;
- ordered `docker exec` steps;
- per-step delays;
- timeouts;
- allowed/required step failures.

A required step that succeeds for baseline and fails for candidate is direct breaking evidence.

### Built-in Docker evidence

The recorder captures:

- image configuration and identity;
- process snapshot;
- filesystem mutations;
- TCP listeners;
- Docker network mode and attachments;
- resource configuration and sampled usage;
- startup/shutdown lifecycle;
- exit status;
- scenario-step outcomes.

Every persisted snapshot has a SHA-256 **Operational ABI fingerprint**.

### Provider-neutral deep evidence

`wabi enrich` merges independent event evidence into a verified snapshot.

| Provider | Input | Role |
|---|---|---|
| **Falco** | JSON alert stream | eBPF/syscall-derived runtime evidence |
| **Tracee** | JSON event stream | eBPF runtime evidence |
| **wabi-native** | RuntimeEvent JSON array | optional native Linux eBPF recorder |
| **Generic** | RuntimeEvent JSONL/array | any third-party/custom sensor |

Normalized semantic identity includes:

```text
category
operation
process
parent_process
target
protocol
direction
```

Provider diagnostics such as source, PID and rule name do **not** define the Operational ABI.

That means equivalent behavior observed by Falco, Tracee or `wabi-native` remains the same ABI fact.

See [`docs/deep-evidence.md`](docs/deep-evidence.md).

---

## Native Linux eBPF recorder

`wabi-native` is a separate optional Linux binary so privileged host observation remains outside the portable core CLI.

The current live vertical slice captures:

- `execve` process execution;
- `openat` file opens;
- outbound `connect`;
- IPv4/IPv6 endpoints;
- process/PID/parent context where available;
- lost perf samples and per-probe diagnostics.

Tracepoint argument offsets are discovered from the running kernel's tracefs metadata rather than hard-coded.

A dedicated GitHub Actions workflow loads the actual eBPF programs on a hosted Linux kernel, generates real process/file/network behavior, verifies the events, and feeds them through the standard snapshot enrichment/fingerprint path.

See [`docs/native-ebpf.md`](docs/native-ebpf.md) and [`SECURITY.md`](SECURITY.md).

---

## Causal Runtime Graph

A deep snapshot can be deterministically converted into an independent graph artifact:

```bash
wabi graph snapshot.deep.json --output graph.json
```

Schema:

```text
wabi.graph/v1alpha1
```

Graph semantics include stable workload/process/file/endpoint/domain/syscall nodes and relationships such as:

```text
spawn
exec
read
write
open
connect:outbound
uses
```

Compare releases:

```bash
wabi graph-diff baseline.graph.json candidate.graph.json
```

The graph has its own fingerprint and remains bound to the source snapshot fingerprint.

See [`docs/causal-runtime-graph.md`](docs/causal-runtime-graph.md).

---

## Target-aware proof engines

### Docker Compose

Current proof surfaces include:

- `read_only` vs filesystem mutations;
- writable mounts/tmpfs;
- memory hard limits;
- `stop_grace_period`;
- capability restrictions.

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --service api \
  baseline.json candidate.json
```

### Kubernetes

Supported workload forms include:

- Pod;
- Deployment;
- StatefulSet;
- DaemonSet;
- ReplicaSet;
- Job;
- CronJob.

Current proof surfaces include:

- read-only root filesystem and writable mounts;
- memory limits;
- termination grace period;
- capabilities;
- `runAsNonRoot` / `runAsUser`;
- privilege escalation restrictions.

Kubernetes JSON is parsed directly. YAML can be normalized client-side with `kubectl`.

```bash
wabi compare-snapshots \
  --target deployment.json \
  --target-kind kubernetes \
  --workload payments \
  --container api \
  baseline.json candidate.json
```

### Kubernetes NetworkPolicy

Workload ABI can prove newly observed outbound dependencies against selecting NetworkPolicies.

Implemented semantics include:

- `matchLabels` / `matchExpressions`;
- additive selected policies;
- `ipBlock`, CIDR and `except`;
- protocols;
- numeric ports and `endPort`;
- deny-all egress.

Unresolved destination selectors, named ports and other insufficiently grounded cases remain `unknown`, not false `BREAKING` verdicts.

```bash
wabi compare-snapshots \
  --target deployment.json \
  --target-kind kubernetes \
  --network-policy egress.json \
  baseline.json candidate.json
```

### Amazon ECS

ECS task definitions are parsed directly with no AWS CLI dependency.

Current proof surfaces include:

- `readonlyRootFilesystem`;
- writable `mountPoints`;
- task/container hard `memory` limits;
- explicit `stopTimeout`;
- Linux capability constraints.

`memoryReservation` is preserved as a soft limit and never treated as a hard compatibility failure.

```bash
wabi compare-snapshots \
  --target task-definition.json \
  --target-kind ecs \
  --container api \
  baseline.json candidate.json
```

### seccomp

Exact observed `category=syscall` requirements can be proven against Docker/OCI-style seccomp profiles.

Conservative action model:

- `ALLOW`, `LOG` → allow;
- `ERRNO`, `KILL*`, `TRAP` → deny;
- `TRACE`, `NOTIFY`, argument-conditional or conflicting rules → unknown.

Unknown never becomes a false deployment blocker.

```bash
wabi compare-snapshots \
  --seccomp-profile seccomp.json \
  baseline.json candidate.json
```

See [`docs/seccomp-proof.md`](docs/seccomp-proof.md) and [`docs/ecs-proof.md`](docs/ecs-proof.md).

---

## Policy

Workload ABI policy composes with environment proof rather than hiding logic in exit codes.

```json
{
  "name": "production",
  "require_scenario": true,
  "require_target": true,
  "deny_severities": ["breaking"],
  "deny_surfaces": ["privilege", "runtime-network"]
}
```

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --policy policy.json \
  baseline.json candidate.json
```

---

## Portable artifacts and outputs

Workload ABI supports:

- human-readable comparison;
- JSON;
- SARIF 2.1.0;
- verified persisted snapshots;
- causal graph artifacts;
- Environment Compatibility Matrix artifacts;
- self-contained HTML dashboard;
- in-toto Statement v1 compatibility attestations;
- CI-safe exit codes.

Offline flow:

```text
build A → record → snapshot A ─┐
                               ├→ compare → environment proofs → matrix → dashboard
build B → record → snapshot B ─┘                         │
                                                        └→ SARIF / in-toto
```

---

## Schema namespaces

Current snapshot writer:

```text
wabi.dev/v1alpha3
```

Backward-compatible persisted `v1alpha2` snapshots and attestations remain readable/verifiable. Published schema meanings are immutable.

Derived artifact namespaces:

```text
wabi.graph/v1alpha1
wabi.graph-diff/v1alpha1
wabi.matrix-config/v1alpha1
wabi.matrix/v1alpha1
```

Public schemas:

```text
schemas/
├── v1alpha2/
├── v1alpha3/
│   ├── snapshot.schema.json
│   ├── comparison.schema.json
│   ├── scenario.schema.json
│   ├── policy.schema.json
│   ├── attestation.schema.json
│   └── runtime-event.schema.json
├── graph/v1alpha1/
│   ├── graph.schema.json
│   └── graph-diff.schema.json
└── matrix/v1alpha1/
    ├── config.schema.json
    └── matrix.schema.json
```

The standalone RuntimeEvent schema is the public interoperability boundary for third-party sensors.

---

## CLI quick reference

```text
wabi compare             BASELINE_IMAGE CANDIDATE_IMAGE
wabi record              IMAGE
wabi enrich              --snapshot SNAPSHOT --events EVENTS --format falco|tracee|generic
wabi compare-snapshots   BASELINE.json CANDIDATE.json
wabi graph               SNAPSHOT.json
wabi graph-diff          BASELINE.graph.json CANDIDATE.graph.json
wabi matrix              --config ENVIRONMENTS.json BASELINE.json CANDIDATE.json
wabi verify-matrix       MATRIX.json
wabi dashboard           --matrix MATRIX.json
wabi attest              COMPARISON.json
wabi verify-attestation  ATTESTATION.json
wabi doctor
wabi version
```

Comparison/matrix exit codes:

- `0` — completed without a breaking verdict;
- `3` — changes found when `--fail-on-change` is requested;
- `4` — a breaking runtime/target/policy/environment regression is proven;
- `1` — execution/data/verification error;
- `2` — CLI usage error.

---

## Install

### Source

Core requirements:

- Go 1.23+
- Docker Engine / Docker Desktop for live Docker recording
- Docker Compose v2 for Compose targets
- `kubectl` only when normalizing Kubernetes YAML

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make check
make build
```

Core binary:

```text
./bin/wabi
```

On Linux, `make build` also produces:

```text
./bin/wabi-native
```

The native provider requires Linux eBPF/tracepoint support, tracefs, and sufficient host privileges.

### Releases

Tagged releases publish AMD64/ARM64 archives for Linux, macOS and Windows.

Linux archives contain:

```text
wabi
wabi-native
```

macOS/Windows archives contain the portable `wabi` CLI.

Releases also publish checksums and public schema bundles.

### Container

Tagged releases publish a multi-architecture core image to:

```text
ghcr.io/riccardomenegazzo/workload-abi
```

Release containers use BuildKit provenance and SBOM generation.

The standard container intentionally remains the portable core CLI. Privileged host eBPF capture stays in the separate Linux binary.

---

## Integrity, trust and dashboard security

An OCI image digest answers:

> Which immutable image bytes are these?

A Workload ABI fingerprint answers:

> Which normalized operational interface did this experiment observe?

Therefore image tags, image IDs, PIDs, provider names, Falco rule names, capture timestamps and noisy point-in-time measurements do not automatically change the Operational ABI.

Persisted evidence is fingerprint-verified before use. Causal graphs and matrices have independent fingerprints and explicit snapshot bindings.

Fingerprints establish **integrity**, not cryptographic authorship. External signing/trust systems such as Sigstore belong to the authenticity layer.

Dashboard-specific safeguards:

- loopback bind by default (`127.0.0.1:8787`);
- non-loopback bind requires `--allow-remote`;
- restrictive Content Security Policy;
- no external CDN/runtime dependency;
- matrix and graph verification before rendering;
- no-cache HTTP responses.

Runtime artifacts may reveal internal file paths, process names, destinations and deployment topology. Treat dashboard exposure accordingly.

See [`SECURITY.md`](SECURITY.md).

---

## Design principles

1. **Observe, do not guess.** Runtime evidence is first-class.
2. **Same experiment, two releases.** Equivalent stimuli are required for meaningful comparison.
3. **Normalize before diffing.** Provider-specific telemetry must not leak into compatibility semantics.
4. **Provider-neutral identity.** Sensor diagnostics do not define the Operational ABI.
5. **Difference is not breakage.** Environment and policy context determine deployment impact.
6. **Unknown is not breaking.** Incomplete evidence must not manufacture confidence.
7. **Explain every verdict.** Every blocker points to concrete normalized evidence.
8. **Keep collection pluggable.** A deeper sensor enriches evidence rather than forking the compatibility engine.
9. **Derive meaning separately.** Graph and matrix artifacts do not mutate the source snapshot.
10. **Preserve old artifacts.** Published schema versions are immutable contracts.
11. **Separate integrity from authenticity.** Fingerprints detect modification; signatures establish trust.

---

## CI proof, not screenshots

The repository continuously proves the architecture end-to-end.

Current workflows verify:

- format, schema validation, `go vet`, race/unit tests and builds;
- Docker experiment recording;
- Falco and Tracee normalization;
- causal graph generation and tamper detection;
- live native eBPF loading/capture on Linux;
- Linux AMD64/ARM64 native packaging;
- Compose target solving;
- Kubernetes NetworkPolicy proof;
- seccomp proof;
- ECS task-definition proof;
- in-toto round trip;
- SARIF generation;
- Environment Compatibility Matrix with four different deployment outcomes;
- matrix tamper detection;
- graph-to-matrix snapshot binding;
- self-contained dashboard export;
- live local dashboard server, `/healthz`, and CSP headers.

The dashboard is therefore a visualization of artifacts already proven by the CLI, not a second source of truth.

---

## Documentation

- [`docs/specification.md`](docs/specification.md) — Operational ABI semantics.
- [`docs/architecture.md`](docs/architecture.md) — implementation boundaries and pipeline.
- [`docs/deep-evidence.md`](docs/deep-evidence.md) — provider contract and normalization.
- [`docs/native-ebpf.md`](docs/native-ebpf.md) — native Linux provider and privilege boundary.
- [`docs/causal-runtime-graph.md`](docs/causal-runtime-graph.md) — causal artifact semantics.
- [`docs/seccomp-proof.md`](docs/seccomp-proof.md) — conservative syscall/seccomp proof.
- [`docs/ecs-proof.md`](docs/ecs-proof.md) — ECS target semantics.
- [`docs/deployability-dashboard.md`](docs/deployability-dashboard.md) — matrix, dashboard and graph correlation.
- [`docs/roadmap.md`](docs/roadmap.md) — maturity gates toward 1.0.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — contribution rules.
- [`SECURITY.md`](SECURITY.md) — threat model and safe execution guidance.

---

## Roadmap

Delivered foundations now form this chain:

```text
portable persisted evidence
        ↓
provider-neutral RuntimeEvent
        ↓
causal runtime graph
        ↓
optional native Linux eBPF recorder
        ↓
Compose / Kubernetes / NetworkPolicy / ECS / seccomp proofs
        ↓
Environment Compatibility Matrix
        ↓
Deployability Dashboard
```

The next major stage is **supply-chain trust integration**: bind image digest, Operational ABI fingerprint, causal graph fingerprint and matrix fingerprint into signed OCI/Sigstore-compatible deployment evidence.

See [`docs/roadmap.md`](docs/roadmap.md).

The 1.0 goal is not “another container security CLI.”

The goal is a portable, independently implementable **Operational ABI for software workloads**.

## License

Apache License 2.0.
