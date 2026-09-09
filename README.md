# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

[![CI](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml)
[![Native eBPF](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/native-ebpf.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/native-ebpf.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Workload ABI (`wabi`) discovers operational breaking changes between container releases.**

It executes two workload versions under equivalent conditions, captures normalized runtime evidence, optionally enriches that evidence with independent deep runtime sensors, derives a causal runtime graph, computes semantic differences, and tests the candidate against the environment where it is expected to run.

```text
                         same experiment
                              |
                 +------------+------------+
                 |                         |
             image:v1                  image:v2
                 |                         |
                 v                         v
            Docker record             Docker record
                 |                         |
          snapshot / fingerprint     snapshot / fingerprint
                 |                         |
       optional deep evidence      optional deep evidence
   Falco / Tracee / wabi-native   Falco / Tracee / custom
                 |                         |
                 +------------+------------+
                              |
                  semantic diff + causal graph
                              |
                 +------------+------------+
                 |                         |
          Compose / Kubernetes           policy
                 |                         |
                 +------------+------------+
                              |
                              v
              COMPATIBLE | CHANGED | BREAKING
                              |
                     JSON / SARIF / in-toto
```

This is **not** an SBOM scanner, vulnerability scanner, attack simulator, or raw log differ.

It answers a different question:

> **Did this release change its operational interface, why did it change, and will that change break the environment where it runs?**

## Why this exists

A release can keep the same API, pass every unit test, build successfully, have fewer CVEs, and still break production because it now:

- writes to a path that was previously read-only;
- opens a new listener;
- contacts a new external dependency;
- starts a helper process;
- needs root or an additional Linux capability;
- exceeds a memory limit;
- stops responding correctly to the same runtime stimulus;
- takes longer than the deployment shutdown grace period;
- introduces a new syscall/kernel assumption.

Traditional SemVer does not describe these changes. Workload ABI treats them as changes to an **Operational ABI**.

## The 30-second demo

Record the same experiment against two releases:

```bash
wabi record --scenario scenario.json app:1.8.3 > baseline.json
wabi record --scenario scenario.json app:1.8.4 > candidate.json
```

Optionally enrich both snapshots with real deep runtime evidence. Falco is one supported provider:

```bash
wabi enrich \
  --snapshot baseline.json \
  --events falco-baseline.jsonl \
  --format falco \
  --output baseline.deep.json

wabi enrich \
  --snapshot candidate.json \
  --events falco-candidate.jsonl \
  --format falco \
  --output candidate.deep.json
```

Or capture normalized evidence directly on Linux with the optional native provider:

```bash
sudo wabi-native record \
  --duration 10s \
  --output native-events.json \
  --stats-output native-stats.json

wabi enrich \
  --snapshot candidate.json \
  --events native-events.json \
  --format generic \
  --output candidate.native.json
```

Compare snapshots against a deployment target:

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --service api \
  baseline.deep.json candidate.deep.json
```

Example:

```text
WORKLOAD ABI
================================================================
app:1.8.3 -> app:1.8.4
scenario: compatibility-smoke
target:   compose:compose.yaml#api

RUNTIME-NETWORK
  + [WARNING] new deep runtime behavior observed:
    network/connect process=/usr/bin/app
    target=telemetry.example.com:443 protocol=tcp direction=outbound

FILESYSTEM
  ~ [BREAKING] candidate introduces a new filesystem mutation outside
    writable Compose mounts while read_only is enabled

LIFECYCLE
  ~ [BREAKING] observed shutdown duration exceeds target grace period

----------------------------------------------------------------
RUNTIME COMPATIBILITY: BREAKING
```

That is the core distinction:

> **A difference is not automatically a breaking change.**
>
> Workload ABI separates observed evidence from compatibility judgment.

## What works today

### Equivalent experiments

Docker is the reference execution backend. A JSON scenario can apply identical inputs to baseline and candidate:

- environment variables;
- command override;
- ordered `docker exec` steps;
- per-step delays;
- timeouts;
- expected/allowed failures.

A required scenario step that succeeds for baseline and fails for candidate is direct evidence of a breaking regression.

### Portable runtime evidence

The built-in Docker recorder captures:

- image configuration and identity;
- process snapshot;
- filesystem mutations;
- TCP listeners;
- Docker network mode and network attachments;
- resource configuration and sampled usage;
- startup/shutdown lifecycle;
- exit status;
- scenario-step outcomes.

Every persisted snapshot has a SHA-256 **Operational ABI fingerprint**.

The fingerprint represents normalized compatibility-relevant behavior. It deliberately does not pretend to be an OCI image digest.

### Provider-neutral deep runtime evidence

`wabi enrich` can merge event-level runtime evidence into a verified snapshot:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events events.jsonl \
  --format falco|tracee|generic \
  --output snapshot.deep.json
```

Current evidence providers:

| Provider | Input | Role |
|---|---|---|
| **Falco** | JSON alert stream | eBPF/syscall-derived runtime evidence |
| **Tracee** | JSON event stream | eBPF runtime evidence |
| **wabi-native** | normalized RuntimeEvent JSON array | optional native Linux eBPF recorder |
| **Generic** | RuntimeEvent JSONL/array | any custom or third-party sensor |

Docker, Falco, Tracee, and the native provider are not embedded into compatibility semantics. They are evidence sources.

The provider-neutral `RuntimeEvent` semantic identity includes:

```text
category
operation
process
parent_process
target
protocol
direction
```

while provider diagnostics such as:

```text
source
pid
rule
```

are intentionally excluded from semantic identity and the operational fingerprint.

That means the same file open observed by Falco with PID `42`, Tracee with PID `9001`, or the native recorder with another PID is still the same Operational ABI fact.

See [`docs/deep-evidence.md`](docs/deep-evidence.md).

### Native Linux eBPF recorder

`wabi-native` is a separate optional binary. It keeps privileged host observation out of the portable core CLI while emitting the same public `RuntimeEvent` contract.

The v0.6 live vertical slice captures:

- process execution through `sys_enter_execve`;
- file opens through `sys_enter_openat`;
- outbound socket connects through `sys_enter_connect`;
- IPv4 and IPv6 endpoint data;
- process/PID/parent context where available;
- perf lost-sample and per-probe diagnostics.

Tracepoint argument offsets are discovered from the running kernel's tracefs metadata instead of being hard-coded.

A dedicated GitHub Actions gate mounts/checks tracefs, loads all three eBPF programs, generates real process/file/network behavior, verifies captured RuntimeEvents, and feeds them through the normal snapshot enrichment/fingerprint pipeline.

See [`docs/native-ebpf.md`](docs/native-ebpf.md) and [`SECURITY.md`](SECURITY.md).

### Causal Runtime Graph

A deep snapshot can be deterministically transformed into an independently versioned graph artifact:

```bash
wabi graph snapshot.deep.json --output graph.json
```

The graph models stable workload/process/file/endpoint/domain/syscall nodes and causal relationships such as:

```text
spawn
read
write
connect:outbound
```

Compare graphs between releases:

```bash
wabi graph-diff baseline.graph.json candidate.graph.json
```

The graph fingerprint is bound to the verified source snapshot fingerprint. Graph artifacts are derived rather than embedded into snapshots so evidence collection and causal interpretation can evolve independently.

See [`docs/causal-runtime-graph.md`](docs/causal-runtime-graph.md).

### Deep diff

New normalized runtime evidence is compared semantically rather than as raw provider JSON.

Examples:

```text
runtime-process / exec
runtime-file    / open
runtime-file    / rename
runtime-network / connect
runtime-network / listen
syscall         / io_uring_setup
```

New deep process/file/network behavior defaults to `warning`; previously unknown syscall evidence defaults to `info` rather than being discarded.

Policy can make those surfaces stricter without changing the core model.

### Target-aware compatibility

Current target adapters:

- Docker Compose;
- Kubernetes Pod;
- Deployment;
- StatefulSet;
- DaemonSet;
- ReplicaSet;
- Job;
- CronJob.

Current constraints include:

- read-only root filesystem and writable mounts;
- memory limits;
- shutdown / termination grace period;
- Linux capability restrictions;
- `runAsNonRoot` / `runAsUser`;
- privilege-escalation restrictions.

Kubernetes JSON is parsed directly. YAML can be normalized client-side with `kubectl`.

### Offline pipeline

Collection and decision do not have to happen in the same process:

```text
build A -> record -> snapshot A --+
                                  +-> compare -> target -> policy -> attestation
build B -> record -> snapshot B --+
```

This allows evidence to be stored as a build artifact and evaluated later against multiple environments or policies.

### Policy

Example:

```json
{
  "name": "production",
  "require_scenario": true,
  "require_target": true,
  "deny_severities": ["breaking"],
  "deny_surfaces": ["privilege", "runtime-network"]
}
```

Run:

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --policy policy.json \
  baseline.json candidate.json
```

Policy violations are explicit evidence in the result, not hidden exit-code logic.

### CI outputs

Workload ABI supports:

- human-readable output;
- JSON;
- SARIF 2.1.0;
- in-toto Statement v1 compatibility attestations;
- CI-safe exit codes.

## Schema versions

Current writer schema:

```text
wabi.dev/v1alpha3
```

`v1alpha3` adds provider-neutral deep runtime events.

Persisted `v1alpha2` snapshots and attestations remain readable/verifiable. The published meaning of `v1alpha2` was not changed.

The causal graph has an independent schema namespace:

```text
wabi.graph/v1alpha1
```

Public schemas include:

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
└── graph/v1alpha1/
    ├── graph.schema.json
    └── graph-diff.schema.json
```

The standalone runtime-event schema is the interoperability contract for third-party sensors.

## Install

### Source

Core requirements:

- Go 1.23+
- Docker Engine / Docker Desktop for live Docker recording
- Docker Compose v2 for Compose targets
- `kubectl` only for Kubernetes YAML normalization

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make check
```

Core binary:

```text
./bin/wabi
```

On Linux, `make build` also builds:

```text
./bin/wabi-native
```

Or explicitly:

```bash
make build-native
```

The native provider additionally requires Linux eBPF/tracepoint support, tracefs, and sufficient host privilege. See [`docs/native-ebpf.md`](docs/native-ebpf.md).

### Release archives

Tagged releases publish AMD64/ARM64 archives for Linux, macOS, and Windows.

Linux archives contain:

```text
wabi
wabi-native
```

macOS and Windows archives contain only the portable `wabi` CLI.

Every release also publishes checksums and the public schema bundle.

### Container

Tagged releases publish a multi-architecture core image to:

```text
ghcr.io/riccardomenegazzo/workload-abi
```

Releases use BuildKit provenance and SBOM generation.

The standard container intentionally remains the portable core CLI; the privileged native host provider is distributed as a Linux binary rather than silently adding host-eBPF privileges to the container path.

Mounting `/var/run/docker.sock` grants the container control of that Docker daemon and must be treated as privileged access. See [`SECURITY.md`](SECURITY.md).

## CLI

### Live compare

```bash
wabi compare app:v1 app:v2
```

### Record

```bash
wabi record --observe 3s --scenario scenario.json app:v1 > snapshot.json
```

### Enrich with Falco

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events falco.jsonl \
  --format falco \
  --output snapshot.deep.json
```

### Enrich with Tracee

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events tracee.jsonl \
  --format tracee \
  --output snapshot.deep.json
```

### Record with native Linux eBPF

```bash
sudo wabi-native record \
  --duration 10s \
  --max-events 10000 \
  --output native-events.json \
  --stats-output native-stats.json

wabi enrich \
  --snapshot snapshot.json \
  --events native-events.json \
  --format generic \
  --output snapshot.native.json
```

### Enrich from any sensor

Emit one normalized RuntimeEvent object per line:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events custom-ebpf.jsonl \
  --format generic
```

A JSON array is accepted by the generic adapter as well.

### Causal graph

```bash
wabi graph snapshot.deep.json --output graph.json
wabi graph-diff baseline.graph.json candidate.graph.json
```

### Offline compare

```bash
wabi compare-snapshots baseline.json candidate.json
```

### Compose target

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --service api \
  baseline.json candidate.json
```

### Kubernetes target

```bash
wabi compare-snapshots \
  --target deployment.yaml \
  --target-kind kubernetes \
  --workload payments \
  --container api \
  baseline.json candidate.json
```

### SARIF

```bash
wabi compare-snapshots \
  --format sarif \
  baseline.json candidate.json > wabi.sarif
```

### Attest

```bash
wabi compare-snapshots \
  --format json \
  baseline.json candidate.json > comparison.json

wabi attest \
  --output compatibility.intoto.json \
  comparison.json
```

Predicate type:

```text
https://wabi.dev/attestation/compatibility/v1alpha1
```

Verify internal consistency:

```bash
wabi verify-attestation compatibility.intoto.json
```

The built-in verifier checks integrity/consistency, not cryptographic authorship. Use an external trust system such as Sigstore when authenticity is required.

### Doctor

```bash
wabi doctor
wabi doctor --json
```

## Fingerprint model

An image digest answers:

> Which immutable image bytes are these?

A Workload ABI fingerprint answers:

> Which normalized operational interface did this experiment observe?

Therefore image tags, image IDs, PIDs, Falco rule names, provider name, capture timestamps, and noisy point-in-time measurements do not automatically change the Operational ABI.

Behavior such as a new file target, network dependency, executable relation, listener, capability or scenario result does.

Persisted snapshots are fingerprint-verified on load. Deep-evidence tampering is rejected before comparison. Causal graph artifacts have their own fingerprint and remain bound to the verified source snapshot fingerprint.

## Exit codes

Comparison commands:

- `0` — completed with no breaking verdict;
- `3` — changes were found and `--fail-on-change` was requested;
- `4` — a breaking runtime regression, target conflict, or policy violation was proven;
- `1` — execution/data/verification error;
- `2` — CLI usage error.

## Design principles

1. **Observe, do not guess.** Runtime evidence is first-class.
2. **Same experiment, two releases.** Equivalent stimuli are required for meaningful comparison.
3. **Normalize before diffing.** Raw Falco, Tracee, Docker, native eBPF, or other telemetry must not leak into compatibility semantics.
4. **Provider-neutral identity.** Sensor-specific diagnostics do not define the Operational ABI.
5. **Difference is not breakage.** Target and policy context determine impact.
6. **Explain every verdict.** Every compatibility decision points to concrete evidence.
7. **Keep collection pluggable.** A deeper sensor should enrich the snapshot, not fork the compatibility engine.
8. **Derive causal meaning separately.** Graph semantics must not mutate evidence artifacts.
9. **Preserve old artifacts.** Published schema versions are immutable contracts.
10. **Separate integrity from authenticity.** Fingerprints detect evidence modification; signatures establish trust.

## Documentation

- [`docs/specification.md`](docs/specification.md) — Operational ABI semantics.
- [`docs/architecture.md`](docs/architecture.md) — internal boundaries and pipeline.
- [`docs/deep-evidence.md`](docs/deep-evidence.md) — provider contract and normalization.
- [`docs/native-ebpf.md`](docs/native-ebpf.md) — native Linux provider and privilege boundary.
- [`docs/causal-runtime-graph.md`](docs/causal-runtime-graph.md) — deterministic causal artifact semantics.
- [`docs/roadmap.md`](docs/roadmap.md) — maturity gates toward 1.0.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — contribution rules.
- [`SECURITY.md`](SECURITY.md) — threat model and safe execution guidance.

## Reproduce the repository deep-evidence proof

```bash
make build

docker build -t wabi-demo:v1 ./examples/demo/v1
docker build -t wabi-demo:v2 ./examples/demo/v2

./bin/wabi record \
  --observe 500ms \
  --scenario ./examples/demo/scenario.json \
  wabi-demo:v1 > /tmp/baseline.json

./bin/wabi record \
  --observe 500ms \
  --scenario ./examples/demo/scenario.json \
  wabi-demo:v2 > /tmp/candidate.json

./bin/wabi enrich \
  --snapshot /tmp/baseline.json \
  --events ./examples/deep/falco-baseline.jsonl \
  --format falco \
  --output /tmp/baseline.deep.json

./bin/wabi enrich \
  --snapshot /tmp/candidate.json \
  --events ./examples/deep/falco-candidate.jsonl \
  --format falco \
  --output /tmp/candidate.deep.json

./bin/wabi compare-snapshots \
  --fail-on-change \
  /tmp/baseline.deep.json /tmp/candidate.deep.json
```

The candidate fixture introduces:

```text
telemetry.example.com:443
```

as a new outbound runtime dependency. CI proves that Falco and Tracee evidence is normalized into provider-neutral facts, while the dedicated Native eBPF workflow proves that the same public boundary can also be populated by live kernel observation.

## Roadmap

Delivered foundations now include:

```text
portable persisted evidence
        ↓
provider-neutral deep RuntimeEvent
        ↓
causal runtime graph
        ↓
optional native Linux eBPF recorder
```

The next major stage is **environment proof expansion**: use the evidence and graph already captured to solve more production constraints, including seccomp, AppArmor, Kubernetes NetworkPolicy, Pod Security, Helm-rendered targets, and richer deployment semantics.

After that, the roadmap moves toward signed OCI/Sigstore compatibility and causal evidence.

See [`docs/roadmap.md`](docs/roadmap.md).

The 1.0 goal is not “another container security CLI.”

The goal is a portable, independently implementable **Operational ABI** for software workloads.

## License

Apache License 2.0.
