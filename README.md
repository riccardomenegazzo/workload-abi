# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

[![CI](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Workload ABI (`wabi`) discovers operational breaking changes between container releases.**

It executes two workload versions under equivalent conditions, captures normalized runtime evidence, enriches that evidence with optional deep runtime sensors, computes a semantic difference, and tests the candidate against the environment where it is expected to run.

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
        optional deep sensors      optional deep sensors
        Falco / Tracee / custom    Falco / Tracee / custom
                 |                         |
                 +------------+------------+
                              |
                        semantic diff
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

> **Did this release change its operational interface, and will that change break the environment where it runs?**

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

Optionally enrich both snapshots with real eBPF-derived runtime evidence. Falco is one supported provider:

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

Compare them:

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

### Deep runtime evidence

`wabi enrich` can merge event-level runtime evidence into a verified snapshot:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events events.jsonl \
  --format falco|tracee|generic \
  --output snapshot.deep.json
```

Current provider adapters:

| Provider | Input | Role |
|---|---|---|
| **Falco** | JSON alert stream | eBPF/syscall-derived runtime evidence |
| **Tracee** | JSON event stream | eBPF runtime evidence |
| **Generic** | Workload ABI RuntimeEvent JSONL/array | any custom sensor |

Docker, Falco, and Tracee are not embedded into compatibility semantics. They are evidence sources.

The provider-neutral `RuntimeEvent` identity includes:

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

That means the same file open observed by Falco with PID `42` and Tracee with PID `9001` is still the same Operational ABI fact.

See [`docs/deep-evidence.md`](docs/deep-evidence.md).

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

Public schemas:

```text
schemas/
├── v1alpha2/
└── v1alpha3/
    ├── snapshot.schema.json
    ├── comparison.schema.json
    ├── scenario.schema.json
    ├── policy.schema.json
    ├── attestation.schema.json
    └── runtime-event.schema.json
```

The standalone runtime-event schema is the interoperability contract for third-party sensors.

## Install

### Source

Requirements:

- Go 1.23+
- Docker Engine / Docker Desktop for live recording
- Docker Compose v2 for Compose targets
- `kubectl` only for Kubernetes YAML normalization

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make check
```

Binary:

```text
./bin/wabi
```

### Container

Tagged releases publish a multi-architecture image to:

```text
ghcr.io/riccardomenegazzo/workload-abi
```

Releases use BuildKit provenance and SBOM generation.

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

### Enrich from any sensor

Emit one normalized RuntimeEvent object per line:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events custom-ebpf.jsonl \
  --format generic
```

A JSON array is accepted by the generic adapter as well.

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

Persisted snapshots are fingerprint-verified on load. Deep-evidence tampering is rejected before comparison.

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
3. **Normalize before diffing.** Raw Falco, Tracee, Docker, or other telemetry must not leak into compatibility semantics.
4. **Provider-neutral identity.** Sensor-specific diagnostics do not define the Operational ABI.
5. **Difference is not breakage.** Target and policy context determine impact.
6. **Explain every verdict.** Every compatibility decision points to concrete evidence.
7. **Keep collection pluggable.** A deeper sensor should enrich the snapshot, not fork the compatibility engine.
8. **Preserve old artifacts.** Published schema versions are immutable contracts.
9. **Separate integrity from authenticity.** Fingerprints detect evidence modification; signatures establish trust.

## Documentation

- [`docs/specification.md`](docs/specification.md) — Operational ABI semantics.
- [`docs/architecture.md`](docs/architecture.md) — internal boundaries and pipeline.
- [`docs/deep-evidence.md`](docs/deep-evidence.md) — provider contract and Falco/Tracee normalization.
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

as a new outbound runtime dependency. The CI gate proves that the Falco event stream is normalized, fingerprinted, persisted, and surfaced as a provider-neutral `runtime-network` Operational ABI change.

A separate CI step runs the same normalization boundary against Tracee JSON.

## Roadmap

The provider-neutral deep-evidence boundary is intentionally implemented **before** a native eBPF collector. The next stages can therefore deepen sensing without redesigning the compatibility engine:

- native optional eBPF recorder;
- outbound DNS dependency normalization;
- process/file/network causal graph;
- syscall/kernel requirement model;
- seccomp and AppArmor solving;
- Kubernetes NetworkPolicy solving;
- Helm-rendered targets;
- signed OCI/Sigstore compatibility attestations;
- external producers/consumers of the RuntimeEvent schema.

The 1.0 goal is not “another container security CLI.”

The goal is a portable, independently implementable **Operational ABI** for software workloads.

## License

Apache License 2.0.
