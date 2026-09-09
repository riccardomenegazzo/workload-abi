# Roadmap

Workload ABI is being developed in layers so new sensors do not destabilize the compatibility model.

## v0.3 — Portable evidence and attestations

Goal: make runtime evidence reusable outside the process that collected it.

- persisted snapshots with fingerprint verification;
- offline comparison;
- policy and target solving on stored evidence;
- public JSON Schemas;
- SARIF output;
- in-toto compatibility statements;
- multi-arch binary/container distribution;
- release SBOM and BuildKit provenance.

## v0.4 — Deep runtime evidence

Goal: move from Docker metadata/point observations to event-level runtime evidence.

Planned evidence providers:

- eBPF process lifecycle;
- file open/read/write/rename/unlink activity;
- TCP connect/accept/listen events;
- DNS queries and resolved destinations;
- capability and syscall requirements;
- stable process identity and parent/child relationships.

The deep recorder must preserve the existing snapshot boundary and remain optional.

## v0.5 — Causal Runtime Graph

Goal: explain *why* a behavior changed, not merely which flat facts changed.

```text
process A
  └─ spawned -> process B
                 ├─ read -> credential file
                 └─ connected -> endpoint
```

Expected capabilities:

- causal process tree;
- file/network edges attributed to the responsible process;
- normalized endpoint identities;
- graph diff between releases;
- explanations that group related mutations into one operational change.

## v0.6 — Environment proof expansion

Goal: solve more production constraints.

- seccomp profiles;
- AppArmor profiles;
- Kubernetes NetworkPolicy;
- Pod Security constraints;
- Helm-rendered Kubernetes targets;
- ECS task definitions;
- richer Compose resource/network semantics.

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
7. at least one external integration consumes Workload ABI output without importing internal Go packages.

The goal is not feature count. The goal is a credible, portable **Operational ABI** that survives implementation changes.
