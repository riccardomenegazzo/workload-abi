# Workload ABI

> **Your API didn't change. Your tests pass. Your image builds. Production can still break.**

[![CI](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml/badge.svg)](https://github.com/riccardomenegazzo/workload-abi/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Workload ABI (`wabi`) discovers operational breaking changes between container releases.**

It executes two workload versions under equivalent conditions, captures normalized runtime evidence, computes a semantic difference, and tests that difference against the environment where the candidate is expected to run.

```text
image:v1 ──┐
           ├─ equivalent experiment ─> runtime evidence ─> semantic diff ─┐
image:v2 ──┘                                                               │
                                                                            ├─> COMPATIBLE
Docker Compose / Kubernetes target ─────────────────────────────────────────┤   CHANGED
policy ──────────────────────────────────────────────────────────────────────┘   BREAKING
```

This is not an SBOM scanner, vulnerability scanner, attack simulator, or log differ.

It answers a different question:

> **Did this release change its operational interface, and will that change break the environment where it runs?**

## The problem

Semantic versioning usually describes an API contract. Container supply-chain tooling can describe packages, provenance, signatures, and vulnerabilities. But a release can preserve all of those expectations and still introduce a production-breaking operational requirement:

- a new filesystem write on a read-only deployment;
- a helper process that did not exist before;
- a new listening port;
- a root or Linux-capability requirement;
- a higher memory envelope;
- a changed shutdown behavior;
- a new runtime failure under the same stimulus;
- an assumption that conflicts with Kubernetes security context.

Workload ABI treats those properties as an **Operational ABI**.

## A 30-second example

```bash
wabi compare \
  --scenario scenario.json \
  --target compose.yaml \
  --service api \
  app:1.8.3 app:1.8.4
```

Example result:

```text
WORKLOAD ABI
================================================================
app:1.8.3 -> app:1.8.4
scenario: compatibility-smoke
target:   compose:compose.yaml#api

FILESYSTEM
  ~ [BREAKING] candidate introduces a new filesystem mutation outside
    writable Compose mounts while read_only is enabled
      after:  A /var/lib/app/cache

LISTENER
  + [WARNING] new listener observed: tcp/8443

LIFECYCLE
  ~ [BREAKING] observed shutdown duration exceeds target grace period

----------------------------------------------------------------
RUNTIME COMPATIBILITY: BREAKING
```

The important distinction is that **a runtime difference is not automatically a breaking change**. A change becomes breaking when Workload ABI can demonstrate failure, a target conflict, or an explicit policy violation.

## What works today

### Runtime evidence

- Docker image orchestration through the local Docker Engine;
- immutable image provenance/config separated from experiment-container state;
- process evidence;
- filesystem mutations;
- TCP listener discovery;
- Docker network mode and attachment metadata;
- CPU/memory/network/block-I/O samples;
- startup/shutdown lifecycle timings;
- exit status;
- ordered scenario-step outcomes;
- bounded step output and execution timeouts;
- stable SHA-256 operational fingerprints.

### Equivalent experiments

Scenarios apply the same inputs to baseline and candidate:

- environment variables;
- command override;
- ordered multi-step `exec` actions;
- delays;
- timeouts;
- expected/allowed failures.

A baseline step that succeeds while the equivalent candidate step fails is direct evidence of an operational regression.

### Environment-aware solving

Current target adapters:

- **Docker Compose**;
- **Kubernetes** Pod, Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, and CronJob.

Current constraints include:

- read-only root filesystem and writable volume paths;
- memory limits;
- shutdown/termination grace periods;
- privilege and capability restrictions;
- Kubernetes `runAsNonRoot` / `runAsUser` expectations;
- privilege-escalation constraints.

Kubernetes JSON is parsed natively. YAML can be normalized client-side with `kubectl` when available.

### CI and interoperability

- human-readable reports;
- JSON comparison documents;
- SARIF 2.1.0 output;
- reusable persisted snapshots;
- offline snapshot comparison;
- policy gates;
- in-toto Statement v1 compatibility attestations;
- public JSON Schemas;
- CI-safe exit codes;
- cross-platform release binaries;
- multi-arch container releases with BuildKit provenance and SBOM.

## Install

### Build from source

Requirements:

- Go 1.23+
- Docker Engine / Docker Desktop for live recording
- Docker Compose v2 for Compose target solving
- `kubectl` only for Kubernetes YAML normalization

```bash
git clone https://github.com/riccardomenegazzo/workload-abi.git
cd workload-abi
make build
./bin/wabi version
```

### Container image

Tagged releases publish a multi-architecture image to:

```text
ghcr.io/riccardomenegazzo/workload-abi
```

For commands that need the local Docker daemon, mounting the Docker socket gives the container control over that daemon and should be treated as privileged access:

```bash
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/riccardomenegazzo/workload-abi:latest \
  doctor
```

See [`SECURITY.md`](SECURITY.md) before using untrusted workload images.

## Compare two live releases

```bash
wabi compare nginx:1.26 nginx:1.27
```

Use a longer observation window when appropriate:

```bash
wabi compare --observe 5s app:v1 app:v2
```

Machine-readable output:

```bash
wabi compare --format json app:v1 app:v2
wabi compare --format sarif app:v1 app:v2 > wabi.sarif
```

`--json` remains an alias for `--format json`.

## Equivalent multi-step scenarios

`scenario.json`:

```json
{
  "name": "checkout-smoke",
  "environment": {
    "APP_MODE": "production-like"
  },
  "steps": [
    {
      "name": "health",
      "after": "250ms",
      "exec": ["/bin/sh", "-c", "test -f /tmp/ready"],
      "timeout": "2s"
    },
    {
      "name": "business-path",
      "exec": ["/app/smoke-test"],
      "timeout": "5s"
    }
  ]
}
```

```bash
wabi compare --scenario scenario.json app:v1 app:v2
```

The scenario name is preserved in the evidence and comparison so the experiment is auditable.

## Record evidence once, compare later

Collection and decision can be separate pipeline stages.

```bash
wabi record --scenario scenario.json app:v1 > baseline.json
wabi record --scenario scenario.json app:v2 > candidate.json
```

Each snapshot contains an operational fingerprint. When a stored snapshot is loaded, Workload ABI recomputes that fingerprint and rejects modified evidence.

Compare without starting the containers again:

```bash
wabi compare-snapshots \
  baseline.json candidate.json
```

Apply a target and policy offline:

```bash
wabi compare-snapshots \
  --target compose.yaml \
  --service api \
  --policy policy.json \
  --format json \
  baseline.json candidate.json > comparison.json
```

This enables a useful CI architecture:

```text
build A -> record -> snapshot A ──┐
                                  ├─> compare -> target -> policy -> attestation
build B -> record -> snapshot B ──┘
```

## Docker Compose target

```bash
wabi compare \
  --scenario scenario.json \
  --target compose.yaml \
  --service api \
  app:v1 app:v2
```

If the Compose file contains exactly one service, `--service` can be omitted.

## Kubernetes target

For JSON manifests:

```bash
wabi compare-snapshots \
  --target deployment.json \
  --target-kind kubernetes \
  --container api \
  baseline.json candidate.json
```

For YAML, Workload ABI can use client-side `kubectl` normalization:

```bash
wabi compare \
  --target deployment.yaml \
  --target-kind kubernetes \
  --workload payments \
  --container api \
  app:v1 app:v2
```

Target analysis is read-only; it must not mutate a cluster.

## Compatibility policy

Example `policy.json`:

```json
{
  "name": "production",
  "require_scenario": true,
  "require_target": true,
  "deny_severities": ["breaking"],
  "deny_surfaces": ["privilege"]
}
```

```bash
wabi compare \
  --scenario scenario.json \
  --target compose.yaml \
  --policy policy.json \
  app:v1 app:v2
```

Policy violations are emitted as explicit `breaking` changes instead of hiding the reason behind a generic failure.

## Compatibility attestations

Produce a JSON comparison first:

```bash
wabi compare-snapshots \
  --format json \
  baseline.json candidate.json > comparison.json
```

Wrap it in an in-toto Statement v1:

```bash
wabi attest \
  --output compatibility.intoto.json \
  comparison.json
```

Verify its internal consistency and optionally pin the expected candidate operational fingerprint:

```bash
wabi verify-attestation \
  --candidate-fingerprint sha256:... \
  compatibility.intoto.json
```

For external signing systems:

```bash
wabi attest --predicate-only comparison.json > predicate.json
```

The built-in verifier checks consistency; it does **not** claim cryptographic authorship. Sign the predicate/statement externally when authenticity is required.

Predicate type:

```text
https://wabi.dev/attestation/compatibility/v1alpha1
```

See [`docs/specification.md`](docs/specification.md).

## Public schemas

Versioned JSON Schemas live under [`schemas/`](schemas/):

```text
schemas/v1alpha2/
├── snapshot.schema.json
├── comparison.schema.json
├── scenario.schema.json
├── policy.schema.json
└── attestation.schema.json
```

They are released alongside the binaries so CI systems and third-party integrations can validate Workload ABI artifacts without importing the Go implementation.

## Doctor

Check local capabilities:

```bash
wabi doctor
wabi doctor --json
```

The command reports availability/version information for relevant local tooling such as Docker, Compose, and Kubernetes tooling.

## Exit codes

For comparison commands:

- `0` — comparison completed with no breaking verdict;
- `3` — changes exist and `--fail-on-change` was requested;
- `4` — a breaking runtime regression, target conflict, or policy violation was detected;
- `1` — execution/data/verification error;
- `2` — CLI usage error.

## Operational fingerprint vs image digest

They are intentionally different.

An **image digest** identifies immutable image content.

A **Workload ABI fingerprint** identifies normalized compatibility-relevant behavior observed under an experiment.

Two different image digests may expose the same Operational ABI. Conversely, a small image change can produce a materially different Operational ABI.

## Design principles

1. **Observe, do not guess.** Runtime evidence is first-class.
2. **Same experiment, two releases.** Comparisons require equivalent stimuli.
3. **Semantic differences, not raw log diffs.** Evidence is normalized before comparison.
4. **Difference is not breakage.** Target constraints and policy decide compatibility impact.
5. **Explain every verdict.** A result must point to concrete evidence.
6. **Keep recorders vendor-neutral.** Evidence providers should not own compatibility semantics.
7. **Preserve provenance.** Image, scenario, target, fingerprints, and policy travel with the result.
8. **Make evidence portable.** Collection and decision can occur in different pipeline stages.
9. **Do not confuse integrity with authenticity.** Fingerprints detect evidence modification; external signatures establish trust.

## Architecture

```text
                         scenario
                            |
               +------------+------------+
               |                         |
           image:v1                   image:v2
               |                         |
               v                         v
        runtime evidence A        runtime evidence B
               |                         |
               v                         v
          snapshot A                 snapshot B
               |                         |
               +------------+------------+
                            |
                      semantic diff
                            |
                   +--------+--------+
                   |                 |
                target             policy
                   |                 |
                   +--------+--------+
                            |
                            v
                 compatibility verdict
                            |
              +-------------+-------------+
              |             |             |
             JSON          SARIF       in-toto
```

More detail:

- [`docs/architecture.md`](docs/architecture.md)
- [`docs/specification.md`](docs/specification.md)

## Reproduce the repository proof

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

./bin/wabi compare-snapshots \
  --target ./examples/demo/compose.yaml \
  --service app \
  --policy ./examples/demo/policy.json \
  /tmp/baseline.json /tmp/candidate.json
```

The included candidate introduces filesystem behavior that conflicts with the read-only target, so Workload ABI proves an environment-specific incompatibility and returns exit code `4`.

## Roadmap

The next technical frontier is deeper runtime evidence while preserving the current compatibility model:

- eBPF process/file/network observation;
- outbound connection and DNS dependency evidence;
- syscall/kernel requirements;
- causal runtime graph rather than flat facts;
- seccomp/AppArmor compatibility;
- Kubernetes NetworkPolicy analysis;
- Helm-rendered target inputs;
- externally signed OCI/Sigstore compatibility attestations.

The goal is a stable **Operational ABI 1.0** that other tools can produce or consume independently of the reference CLI.

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). Public schema and compatibility-rule changes should include tests and documentation.

## Security

See [`SECURITY.md`](SECURITY.md). In particular, Docker socket access and execution of untrusted images must be treated as privileged operations.

## License

Apache License 2.0.
