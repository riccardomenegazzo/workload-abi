# Workload ABI specification

Workload ABI defines an **operational compatibility model** for executable workloads.

An API describes how software communicates with callers. A binary ABI describes how compiled components interact. Workload ABI describes how a running workload interacts with its execution environment.

The project deliberately separates three layers:

1. **evidence** — facts observed while a workload is executed;
2. **semantic change** — normalized differences between two evidence sets;
3. **compatibility** — whether those changes violate a target environment or policy.

A runtime difference is not automatically a breaking change.

## Evidence model

A snapshot is a versioned `wabi.dev/v1alpha2` document containing normalized evidence for one image under one experiment. The current portable evidence surfaces are:

- process commands;
- filesystem mutations;
- TCP listeners;
- image execution configuration;
- container privilege and capability configuration;
- resource configuration and sampled usage;
- Docker network mode and attachments;
- lifecycle timing and exit status;
- ordered scenario-step outcomes.

Collection timestamps, point-in-time statistics, warnings, and step duration/output are useful diagnostics but are intentionally excluded from the stable operational fingerprint where they would introduce incidental nondeterminism.

## Operational fingerprint

Every persisted snapshot carries a SHA-256 fingerprint computed from the normalized compatibility-relevant evidence.

The fingerprint is not an image digest. It answers a different question:

> Which operational interface did this experiment observe?

Loading a persisted snapshot recomputes the fingerprint and rejects the artifact when the evidence has been modified without updating the fingerprint.

## Equivalent experiment rule

A comparison is meaningful only when both releases are exercised under equivalent conditions.

A scenario may define:

- environment variables;
- a command override;
- ordered `docker exec` steps;
- per-step delays;
- per-step timeouts;
- whether a failing step is expected.

When persisted snapshots declare different scenario names, `compare-snapshots` rejects the comparison instead of silently treating them as equivalent.

## Semantic change classes

Changes are normalized into a `surface`, `kind`, `severity`, and explanation.

Current severities are:

- `info` — evidence changed but no default compatibility risk is asserted;
- `warning` — the operational interface changed in a way that may matter;
- `breaking` — direct evidence demonstrates failure or an expanded requirement that violates a compatibility rule.

The aggregate verdict is:

- `COMPATIBLE` — no material difference was found;
- `CHANGED` — one or more non-breaking changes were found;
- `BREAKING` — at least one breaking change, target conflict, scenario regression, or policy violation was proven.

## Target-aware compatibility

Workload ABI does not classify every new behavior as breaking. It combines candidate evidence with target constraints.

Examples:

- a new filesystem write is breaking against a read-only root filesystem unless a writable mount covers that path;
- observed memory usage above a hard target limit is breaking;
- a shutdown duration beyond the configured grace period is breaking;
- an added capability conflicts with an environment that drops all capabilities;
- a root requirement conflicts with a Kubernetes `runAsNonRoot` target.

Docker Compose and Kubernetes are target adapters. They do not change the evidence model.

## Policy layer

Policies allow organizations to make compatibility stricter without changing the core diff semantics.

A policy can require a scenario or target, deny severities/surfaces/change kinds, or cap the number of accepted changes.

Policy violations are explicit `policy` changes with `breaking` severity, so machine and human outputs preserve the reason for the final verdict.

## Persisted evidence

`wabi record` emits a snapshot that can be stored as a CI artifact. `wabi compare-snapshots` evaluates two stored snapshots without starting Docker containers again.

This makes collection and decision separate pipeline stages:

```text
build image A ──> record ──> snapshot A ──┐
                                         ├─> compare ─> target ─> policy ─> verdict
build image B ──> record ──> snapshot B ──┘
```

## Attestation

A completed comparison can be wrapped in an in-toto Statement v1 using the predicate type:

```text
https://wabi.dev/attestation/compatibility/v1alpha1
```

The attestation subject is the candidate workload and its digest is derived from the candidate operational fingerprint. This binds the compatibility result to the observed operational interface rather than pretending that the fingerprint is the OCI image digest.

The built-in verifier validates internal consistency. Cryptographic signing is intentionally a separate concern: `--predicate-only` exists so Sigstore, OCI attestation tooling, or other signing systems can sign the Workload ABI predicate without the core project owning a proprietary trust model.

## Schema lifecycle

Public schemas live under `schemas/<version>/`.

`v1alpha2` is experimental. Until a stable `v1` exists, incompatible schema changes are allowed only by introducing a new schema version. Existing published schemas are not silently rewritten to mean something different.

## Non-goals

Workload ABI is not an SBOM, vulnerability scanner, attack simulator, runtime security product, or replacement for integration tests.

Its narrow question is:

> **Did this release change its operational interface, and does that change break the environment where it is expected to run?**
