# Roadmap

Workload ABI is developed in layers so new sensors do not destabilize the compatibility model.

## v0.3 — Portable evidence and attestations ✅

Goal: make runtime evidence reusable outside the process that collected it.

Delivered:

- persisted snapshots with fingerprint verification;
- offline comparison;
- policy and target solving on stored evidence;
- public JSON Schemas;
- SARIF output;
- in-toto compatibility statements;
- multi-arch binary/container distribution;
- release SBOM and BuildKit provenance.

## v0.4 — Provider-neutral deep runtime evidence ✅

Goal: make event-level runtime evidence portable across independent sensors before committing the compatibility model to a native collector.

Delivered:

- `wabi.dev/v1alpha3` with backward-compatible v1alpha2 loading;
- public `RuntimeEvent` interoperability schema;
- Falco JSON adapter;
- Tracee JSON adapter;
- generic JSONL/JSON-array provider;
- deep process/file/network/syscall normalization;
- provider-neutral semantic identity;
- deep evidence in operational fingerprints;
- semantic deep-event diffing;
- `wabi enrich` for persisted snapshots;
- end-to-end Falco and Tracee fixture gates;
- deep-evidence tamper detection.

The important architectural result is that Docker can remain an experiment runner while eBPF systems become interchangeable evidence providers.

## v0.5 — Native deep recorder + Causal Runtime Graph

Goal: capture richer evidence directly and explain *why* behavior changed rather than only which normalized facts changed.

Planned:

- optional native Linux eBPF provider;
- process lifecycle events;
- file open/read/write/rename/unlink activity;
- TCP connect/accept/listen evidence;
- DNS queries and resolved destinations;
- syscall/capability requirements;
- stable process identity and parent/child relationships;
- causal process/file/network graph;
- graph diff between releases;
- explanations grouping related mutations into one operational change.

The native recorder must emit the same public RuntimeEvent/graph boundary rather than create a second compatibility engine.

## v0.6 — Environment proof expansion

Goal: solve more production constraints.

- seccomp profiles;
- AppArmor profiles;
- Kubernetes NetworkPolicy;
- Pod Security constraints;
- Helm-rendered Kubernetes targets;
- ECS task definitions;
- richer Compose resource/network semantics;
- use observed outbound dependencies to prove NetworkPolicy conflicts.

## v0.7 — Supply-chain trust integration

Goal: make compatibility evidence a first-class signed deployment artifact.

- OCI referrer/attestation publishing;
- Sigstore signing and verification examples;
- image-digest + operational-fingerprint binding;
- CI policy examples for GitHub Actions and other systems;
- provenance linking scenario, target, evidence and comparison.

## 1.0 maturity criteria

Workload ABI should not declare a stable 1.0 specification until all of the following are true:

1. snapshot/comparison schemas have a documented compatibility policy;
2. at least two independent evidence providers can produce compatible normalized evidence;
3. target solving supports Docker Compose and Kubernetes with a stable rule model;
4. persisted evidence and attestation formats have interoperability tests;
5. nondeterminism is measured and documented;
6. the project has a corpus of reproducible compatibility scenarios;
7. at least one external integration consumes Workload ABI output without importing internal Go packages;
8. deep provider equivalence is tested against the same workload behavior;
9. a native deep recorder, if shipped, uses the same public evidence model as external providers.

The goal is not feature count. The goal is a credible, portable **Operational ABI** that survives implementation and sensor changes.
