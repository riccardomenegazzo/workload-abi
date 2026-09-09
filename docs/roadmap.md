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

## v0.6 — Native optional deep recorder ✅

Goal: prove that Workload ABI can capture Linux runtime evidence directly without creating a second compatibility model.

Delivered vertical slice:

- standalone optional Linux `wabi-native` binary;
- native eBPF tracepoint programs built with `cilium/ebpf`;
- runtime discovery of tracepoint argument offsets from tracefs metadata;
- `execve` process evidence with executable path;
- `openat` file evidence with pathname;
- outbound `connect` evidence with IPv4/IPv6 endpoint decoding;
- process/PID/parent context where available;
- bounded collection window and unique-event cap;
- perf lost-sample accounting;
- per-probe load/attach diagnostics;
- direct normalization into the existing public `RuntimeEvent` contract;
- ingestion through the existing `wabi enrich --format generic` path;
- live-kernel GitHub Actions proof that all three probes attach and capture real events;
- end-to-end proof that native evidence survives snapshot enrichment and fingerprinting;
- Linux AMD64 and ARM64 release builds for the native provider.

The architectural gate is therefore complete: **native collection changes the evidence source, not compatibility semantics**.

The following deeper native coverage remains intentionally incremental rather than being required to validate the provider boundary:

- fork/clone/exit lifecycle;
- file read/write/rename/unlink semantics;
- TCP accept/listen;
- DNS queries and resolved destinations;
- syscall/capability requirement evidence;
- container/cgroup attribution;
- cross-provider equivalence corpus for the same live workload.

See [`native-ebpf.md`](native-ebpf.md).

## v0.7 — Environment proof expansion 🚧

Goal: solve production constraints using observed evidence and the causal model, while issuing `BREAKING` only when an incompatibility is actually provable.

Delivered so far:

- Kubernetes workload namespace and Pod-template label identity;
- Kubernetes NetworkPolicy selection using `matchLabels` and `matchExpressions`;
- additive egress-policy semantics;
- `ipBlock` CIDR and `except` proof;
- protocol, numeric port and `endPort` proof;
- conservative unresolved handling for destination selectors, named ports, and hostname/IP mismatches;
- end-to-end proof that a newly observed outbound dependency can be rejected by the target NetworkPolicy;
- Docker/OCI-style seccomp profile parsing;
- conservative seccomp action classification (`allow`, `deny`, `unknown`);
- release-regression proof for newly observed exact `category=syscall` requirements;
- explicit unknown handling for argument-conditional rules, `TRACE`, `NOTIFY`, and profile conditions;
- end-to-end seccomp proof from persisted snapshot + public RuntimeEvent enrichment to `syscall / target-conflict`.

Still planned:

- AppArmor profiles;
- broader exact syscall identity without weakening published v1alpha3 semantics;
- Pod Security constraints beyond the current container security-context checks;
- Helm-rendered Kubernetes targets;
- ECS task definitions;
- richer Compose resource/network semantics;
- destination workload identity for NetworkPolicy pod/namespace selector proof;
- map causal graph edges to the exact target constraint they violate.

The v0.7 design rule is: **unknown is not breaking**. Target solvers must preserve uncertainty rather than manufacture confidence from incomplete runtime evidence.

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
