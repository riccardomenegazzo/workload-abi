# Architecture

Workload ABI separates collection from compatibility semantics so that deeper runtime observers can be introduced without changing the CLI or report model.

## Pipeline

```text
image A ---------------------> Docker recorder ------------------+
                                                                |
                                                                v
                                                         Snapshot A
                                                                |
                                                                +-----> Semantic diff -----> Verdict
                                                                |
                                                         Snapshot B
                                                                ^
                                                                |
image B ---------------------> Docker recorder ------------------+
```

## Snapshot model

The v0.1 snapshot records:

- image identity and capture timestamp;
- observed process commands;
- filesystem mutations relative to the image layer;
- image user, entrypoint, command, working directory, stop signal, exposed ports, and healthcheck;
- container privilege/capability/resource configuration;
- container exit status;
- startup and shutdown timings;
- point-in-time Docker stats.

Collection failures for optional surfaces are recorded as warnings instead of discarding the entire experiment. This is important for short-lived or minimal images where `docker top` or `docker stats` can legitimately become unavailable before collection finishes.

## Semantic diff

The diff engine does not compare raw JSON. Each surface is normalized into stable sets or scalar facts first. Changes are classified as:

- `info`: operational difference with low default risk;
- `warning`: a potentially relevant compatibility difference;
- `breaking`: a difference that demonstrates failure or a materially expanded execution requirement.

The current breaking rules are intentionally conservative. v0.1 only marks evidence as breaking where the tool has direct evidence (for example, a candidate that fails under the same workflow). Future environment-aware analysis will be able to prove more incompatibilities.

## Why Docker primitives first

A full eBPF recorder is a major subsystem. Starting with Docker-native primitives provides a useful vertical slice immediately while keeping the architecture ready for deeper backends.

The recorder interface will evolve toward independent observation providers:

```text
Recorder
├── Docker metadata
├── process observer
├── filesystem observer
├── network observer
├── kernel observer
└── lifecycle observer
```

The normalized snapshot remains the boundary between collection and comparison.

## Planned causal graph

The longer-term model is not a flat allow-list. It is a causal runtime graph:

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

Comparing two graph versions makes it possible to express differences such as "the main process now spawns a helper that reads a credential and connects to a new domain", rather than three unrelated events.

## Target environment solver

The next major layer consumes deployment constraints from Docker Compose first, followed by Kubernetes and other runtimes.

Conceptually:

```text
candidate operational requirements
                 X
target environment constraints
                 =
compatibility result
```

Examples include:

- new writable paths against a read-only root filesystem;
- new capabilities against `cap_drop: ALL`;
- new ports against network policy;
- higher memory requirements against hard limits;
- shutdown regressions against termination grace periods.

## Non-goals

Workload ABI is not intended to become:

- an SBOM generator;
- a vulnerability scanner;
- a generic container sandbox;
- an attack simulator;
- a replacement for runtime security tools.

It consumes runtime evidence to answer a different question: **did this release change its operational interface, and does that change matter to the environment where it will run?**
