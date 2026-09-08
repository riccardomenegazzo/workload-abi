# Research roadmap

## Near term

1. Replace high-frequency Docker exec sampling with a Linux eBPF recorder while preserving the RuntimeGraph contract.
2. Add DNS/process attribution to network behavior.
3. Introduce repeated experiments and confidence scoring.
4. Improve filesystem normalization to distinguish application state from package/runtime noise.
5. Add explicit resource-envelope search: progressively constrain memory/CPU until a minimum viable envelope is found.

## Kubernetes

A Kubernetes solver should understand:

- `securityContext` and Pod Security Standards;
- requests/limits;
- readiness/liveness/startup probes;
- `terminationGracePeriodSeconds`;
- volumes and read-only mounts;
- NetworkPolicy;
- service ports;
- node/kernel architecture constraints.

The intended question remains differential: *does the candidate introduce an operational assumption the existing manifest does not satisfy?*

## OCI attestation

A future Runtime Compatibility Attestation could bind:

- baseline digest;
- candidate digest;
- scenario digest;
- recorder identity/version;
- RuntimeGraph digests;
- compatibility result and evidence.

The attestation should be signable with Sigstore and attachable to an OCI subject without defining a proprietary registry.

## Runtime SemVer research

Longer term, the project can explore whether operational changes can be classified in a SemVer-like language. This is intentionally not part of v0.1: operational compatibility is environment-dependent, so a universal MAJOR/MINOR/PATCH label requires more research than a simple severity mapping.
