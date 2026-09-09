# Workload ABI specification

Workload ABI defines an **operational compatibility model** for executable workloads.

An API describes how software communicates with callers. A binary ABI describes how compiled components interact. Workload ABI describes how a running workload interacts with its execution environment.

The project deliberately separates four layers:

1. **evidence** — facts observed while a workload is executed;
2. **normalization** — provider-specific telemetry mapped into stable operational facts;
3. **semantic change** — differences between two normalized evidence sets;
4. **compatibility** — whether those changes violate a target environment or policy.

A runtime difference is not automatically a breaking change.

## Evidence model

The current writer schema is `wabi.dev/v1alpha3`.

A snapshot contains normalized evidence for one image under one experiment. Portable evidence surfaces include:

- process commands;
- filesystem mutations;
- TCP listeners;
- provider-neutral deep runtime events;
- image execution configuration;
- container privilege and capability configuration;
- resource configuration and sampled usage;
- Docker network mode and attachments;
- lifecycle timing and exit status;
- ordered scenario-step outcomes.

Collection timestamps, point-in-time statistics, warnings, provider rule names, PIDs, and step duration/output can be useful diagnostics but are excluded from stable semantic identity where they would introduce incidental nondeterminism.

## Deep runtime event

`v1alpha3` introduces `RuntimeEvent`, the interoperability boundary for event-level runtime sensors.

Semantic fields are:

- `category`;
- `operation`;
- `process`;
- `parent_process`;
- `target`;
- `protocol`;
- `direction`.

Diagnostic fields are:

- `source`;
- `pid`;
- `rule`.

Diagnostic fields do not participate in the event semantic key or Operational ABI fingerprint. Therefore identical behavior can normalize to the same fact when collected by different sensors.

Current adapters understand Falco JSON output, Tracee JSON output, and a generic RuntimeEvent JSONL/array protocol. The public generic schema lives at `schemas/v1alpha3/runtime-event.schema.json`.

## Operational fingerprint

Every persisted snapshot carries a SHA-256 fingerprint computed from normalized compatibility-relevant evidence.

The fingerprint is not an image digest. It answers a different question:

> Which operational interface did this experiment observe?

Loading a persisted snapshot recomputes the fingerprint and rejects the artifact when fingerprinted evidence has been modified without updating the fingerprint.

Image reference/digest metadata is deliberately separate from the operational fingerprint. Two different image artifacts may expose the same Operational ABI.

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

Deep enrichment is a post-collection operation. Enrichment must not change the scenario identity of the underlying experiment.

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

### Deep evidence defaults

New deep `process`, `file`, `network`, or `security` behavior defaults to `warning`.

Unknown syscall evidence is retained as `syscall/<name>` and defaults to `info`. It is not discarded simply because the reference implementation does not yet understand the syscall's higher-level semantics.

Removal of previously observed deep behavior defaults to `info`.

Policy may tighten these defaults without changing core comparison semantics.

## Target-aware compatibility

Workload ABI does not classify every new behavior as breaking. It combines candidate evidence with target constraints.

Examples:

- a new filesystem write is breaking against a read-only root filesystem unless a writable mount covers that path;
- observed memory usage above a hard target limit is breaking;
- a shutdown duration beyond the configured grace period is breaking;
- an added capability conflicts with an environment that drops all capabilities;
- a root requirement conflicts with a Kubernetes `runAsNonRoot` target.

Docker Compose and Kubernetes are target adapters. They do not change the evidence model.

Future target adapters can consume deep runtime evidence. For example, an observed outbound network dependency can be evaluated against Kubernetes NetworkPolicy without changing how the dependency was originally collected.

## Policy layer

Policies allow organizations to make compatibility stricter without changing the core diff semantics.

A policy can require a scenario or target, deny severities/surfaces/change kinds, or cap the number of accepted changes.

Policy violations are explicit `policy` changes with `breaking` severity, so machine and human outputs preserve the reason for the final verdict.

Deep surfaces use names such as `runtime-network`, `runtime-file`, `runtime-process`, and `runtime-syscall`, making them directly gateable by existing policy semantics.

## Persisted evidence

`wabi record` emits a snapshot that can be stored as a CI artifact. `wabi enrich` merges provider-specific event streams into a verified snapshot. `wabi compare-snapshots` evaluates two stored snapshots without starting Docker containers again.

```text
build image A -> record -> snapshot A -> optional enrich --+
                                                            +-> compare -> target -> policy -> verdict
build image B -> record -> snapshot B -> optional enrich --+
```

The enrichment layer is optional. Docker-native snapshots remain valid Workload ABI artifacts without Falco, Tracee, or another deep sensor.

## Attestation

A completed comparison can be wrapped in an in-toto Statement v1 using the predicate type:

```text
https://wabi.dev/attestation/compatibility/v1alpha1
```

The attestation subject is the candidate workload and its digest is derived from the candidate operational fingerprint. This binds the compatibility result to the observed operational interface rather than pretending that the fingerprint is the OCI image digest.

The built-in verifier validates internal consistency. Cryptographic signing is intentionally a separate concern: `--predicate-only` exists so Sigstore, OCI attestation tooling, or other signing systems can sign the Workload ABI predicate without the core project owning a proprietary trust model.

`v1alpha2` comparison/attestation predicates remain accepted for backward compatibility.

## Schema lifecycle

Public schemas live under `schemas/<version>/`.

Published versions are immutable contracts. The project did **not** add `runtime_events` to `v1alpha2`; instead it introduced `v1alpha3`.

The current compatibility promise is:

- new writers emit `v1alpha3`;
- readers accept `v1alpha2` and `v1alpha3`;
- enriching a `v1alpha2` snapshot produces a new `v1alpha3` snapshot and a new fingerprint;
- `v1alpha2` artifacts cannot claim `runtime_events`;
- an unsupported future schema fails closed rather than being silently misinterpreted.

Until a stable `v1` exists, incompatible schema changes are allowed only by introducing another explicit schema version.

## Provider neutrality

Evidence providers must not own compatibility semantics.

A provider is responsible for turning telemetry into normalized facts. It should not decide whether an event is breaking for a given target. That decision belongs to semantic diff, target solving, and policy.

This allows Docker, Falco, Tracee, a future native eBPF recorder, or a third-party implementation to coexist without forking the Operational ABI model.

## Non-goals

Workload ABI is not an SBOM, vulnerability scanner, attack simulator, runtime security product, or replacement for integration tests.

Its narrow question is:

> **Did this release change its operational interface, and does that change break the environment where it is expected to run?**
