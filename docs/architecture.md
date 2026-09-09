# Architecture

Workload ABI separates **evidence collection**, **normalization**, **comparison**, **target solving**, **policy**, and **attestation**. The normalized snapshot is the stable boundary: deeper observers can be added without coupling compatibility semantics to one runtime sensor.

## Pipeline

```text
                         scenario
                            |
             +--------------+--------------+
             |                             |
         image A                         image B
             |                             |
             v                             v
      Docker experiment             Docker experiment
             |                             |
             v                             v
        Snapshot A                    Snapshot B
             |                             |
             +--------------+--------------+
                            |
                      semantic diff
                            |
                   +--------+--------+
                   |                 |
             target solver       policy gate
          Compose / Kubernetes       |
                   +--------+--------+
                            |
                            v
                 compatibility verdict
                   /        |         \
              human        JSON       SARIF
                              \
                               +----> in-toto statement
```

Snapshots can be compared immediately or persisted and evaluated later. That split is intentional: collection can happen close to the build, while target solving, policy, review, and attestation can happen in a separate CI stage.

## Evidence schema

The current schema is `wabi.dev/v1alpha2`. A snapshot records:

- immutable image identity and configuration;
- experiment/scenario provenance;
- observed process commands;
- filesystem mutations relative to the image layer;
- runtime TCP listeners when container procfs is readable;
- Docker network mode and attached networks;
- scenario-step commands, bounded output, duration, exit code, success and allowed-failure state;
- runtime privilege/capability/resource configuration;
- startup/shutdown lifecycle measurements;
- point-in-time Docker stats;
- optional-surface collection warnings;
- a stable SHA-256 operational-evidence fingerprint.

Public machine-readable schemas are versioned under `schemas/`.

## Operational fingerprint

The fingerprint identifies normalized compatibility-relevant evidence, not the image bytes.

It deliberately excludes volatile measurements such as capture timestamps, CPU percentages, diagnostic warnings, exact lifecycle timings, and scenario output where those values would introduce incidental nondeterminism.

Persisted snapshots are verified on load by recomputing the fingerprint. A snapshot whose evidence changed without a matching fingerprint is rejected before comparison.

## Equivalent experiment engine

A scenario is applied identically to baseline and candidate. It can define environment variables, a command override and ordered `docker exec` steps with delays, timeouts and allowed-failure semantics.

Step outcomes are first-class evidence. A required step that succeeds for the baseline and fails for the candidate is a directly demonstrated compatibility regression and therefore `BREAKING`.

## Semantic diff

Raw JSON is not diffed. Each evidence surface is normalized first. Changes use three default severities:

- `info`: low-risk operational difference;
- `warning`: potentially relevant contract change;
- `breaking`: demonstrated failure or expanded requirement that violates a known invariant.

Current surfaces include processes, files, image ports, runtime listeners, network metadata, image configuration, capabilities, lifecycle and scenario outcomes.

## Target solvers

Target solvers answer a different question from the semantic diff: **does the candidate evidence conflict with the environment where the workload is expected to run?**

### Docker Compose

Compose is normalized with `docker compose config --format json`. The solver currently evaluates read-only rootfs/writable mounts, memory limits, shutdown grace periods and capability constraints.

### Kubernetes

Kubernetes JSON manifests are parsed natively; YAML is rendered to JSON with client-side `kubectl` normalization when available. Supported workload kinds are Pod, Deployment, StatefulSet, DaemonSet, ReplicaSet, Job and CronJob.

Current constraints include:

- `readOnlyRootFilesystem` and writable volume mounts;
- memory limits;
- `terminationGracePeriodSeconds`;
- `runAsNonRoot` / `runAsUser`;
- dropped capabilities;
- privilege-escalation restrictions.

Target analysis is read-only and is not intended to mutate a Kubernetes cluster.

## Policy gate

Policy is deliberately applied after evidence and target solving. A policy can require scenario/target context, reject severities, surfaces or change kinds, and enforce a maximum change count.

A policy failure is not hidden state: it is appended to the result as explicit `policy-violation` evidence and promotes the verdict to `BREAKING`.

## Offline evidence boundary

`record` serializes a verified snapshot; `compare-snapshots` consumes two verified snapshots and reuses the same diff/target/policy pipeline as a live comparison.

```text
CI build stage                         CI decision stage
--------------                         -----------------
image A -> record -> snapshot A ----+
                                      +-> verify fingerprints
image B -> record -> snapshot B ----+          |
                                                v
                                         semantic diff
                                                |
                                          target + policy
                                                |
                                             verdict
```

This allows snapshots to be stored as build artifacts, reviewed, reproduced, or evaluated against multiple target environments without rerunning the workload.

## Attestation boundary

A comparison can be wrapped in an in-toto Statement v1 using the predicate type:

```text
https://wabi.dev/attestation/compatibility/v1alpha1
```

The subject is the candidate workload and its subject digest is derived from the candidate operational fingerprint. This deliberately does not masquerade as the OCI image digest.

The built-in verifier checks:

- statement and predicate types;
- predicate schema version;
- candidate/fingerprint presence;
- subject/candidate consistency;
- subject digest/fingerprint consistency;
- optional expected fingerprint pinning.

This provides integrity/consistency, not authorship. External systems such as Sigstore can sign the statement or the `--predicate-only` output when trust/authenticity is required.

## Recorder evolution

Docker-native primitives provide the portable vertical slice. The intended deeper architecture is provider-oriented:

```text
Recorder
├── Docker metadata
├── process observer
├── filesystem observer
├── network/DNS observer
├── syscall/capability observer
├── kernel observer
└── lifecycle observer
```

The next major recorder depth is eBPF. It should enrich the same snapshot boundary rather than replace compatibility semantics.

## Causal graph direction

The longer-term evidence model can evolve from flat normalized sets into a causal runtime graph:

```text
workload
  |
  +-- spawned --> process
  |                 |
  |                 +-- read ------> file
  |                 +-- wrote -----> file
  |                 +-- connected -> endpoint
  |
  +-- listened --> port
```

This enables explanations such as “the main process now spawns a helper that reads a credential and connects to a new domain” instead of three unrelated events.

## Distribution

Tagged releases produce:

- Linux/macOS/Windows binaries for amd64 and arm64;
- checksums;
- the public schema bundle;
- a Linux multi-architecture GHCR image.

The container release is built with BuildKit provenance and SBOM generation so Workload ABI's own distribution follows the same supply-chain principles the project expects from modern container workflows.

## Non-goals

Workload ABI is not an SBOM generator, vulnerability scanner, generic container sandbox, attack simulator, or replacement for runtime security products.

Its question remains narrow: **did this release change its operational interface, and does that change matter to the environment where it will run?**
