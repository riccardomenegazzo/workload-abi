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

## v0.5 — Causal Runtime Graph ✅

Goal: explain *why* behavior changed rather than only which normalized facts changed.

Delivered:

- independently versioned `wabi.graph/v1alpha1` graph artifact;
- graph fingerprint bound to the verified source snapshot fingerprint;
- stable workload/process/file/endpoint/domain/syscall nodes;
- deterministic semantic edges such as `spawn`, `read`, `write`, and `connect:outbound`;
- `wabi graph` generation from persisted deep evidence;
- `wabi graph-diff` between releases;
- grouped causal explanations for new process chains;
- standalone dependency explanations;
- public graph and graph-diff JSON Schemas;
- graph tamper detection;
- end-to-end proof of a new helper process reading a credential and opening an outbound dependency.

The graph is intentionally a **derived artifact**, not a snapshot field. Evidence collection and causal interpretation can therefore evolve independently.

## v0.6 — Native optional deep recorder

Goal: capture richer Linux runtime evidence directly while preserving the public RuntimeEvent boundary proven by external providers.

Planned:

- optional CO-RE/eBPF Linux provider;
- process lifecycle and parent/child identity;
- file open/read/write/rename/unlink activity;
- TCP connect/accept/listen evidence;
- DNS queries and resolved destinations;
- syscall/capability requirements;
- container/cgroup attribution;
- bounded event collection and drop accounting;
- deterministic normalization into the existing `RuntimeEvent` schema;
- equivalence tests against Falco/Tracee/generic evidence for the same behavior.

The native recorder must not create a second compatibility engine.

## v0.7 — Environment proof expansion

Goal: solve more production constraints using the evidence and causal graph already captured.

Planned:

- seccomp profiles;
- AppArmor profiles;
- Kubernetes NetworkPolicy;
- Pod Security constraints;
- Helm-rendered Kubernetes targets;
- ECS task definitions;
- richer Compose resource/network semantics;
- use observed outbound dependencies to prove NetworkPolicy conflicts;
- map causal graph edges to the exact target constraint they violate.

## v0.8 — Supply-chain trust integration

Goal: make compatibility and causal evidence first-class signed deployment artifacts.

Planned:

- OCI referrer/attestation publishing;
- Sigstore signing and verification examples;
- image-digest + operational-fingerprint + graph-fingerprint binding;
- CI policy examples for GitHub Actions and other systems;
- provenance linking scenario, target, snapshot, graph and compatibility decision.

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
9. a native deep recorder, if shipped, uses the same public evidence model as external providers;
10. causal graph artifacts have stable semantics and interoperability tests independent of the recorder implementation.

The goal is not feature count. The goal is a credible, portable **Operational ABI** that survives implementation and sensor changes.
